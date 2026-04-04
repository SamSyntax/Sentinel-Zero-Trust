package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	certmanager "sentinel-zt/data-plane/internal/cert_manager"
	"sentinel-zt/data-plane/internal/config"
	"sentinel-zt/data-plane/internal/grpc"
	"sentinel-zt/data-plane/internal/logger"
	utils "sentinel-zt/data-plane/internal/utils"
	"strings"
	"sync"
	"syscall"
	"time"
)

type IdentityRequest struct {
	ServiceName string `json:"serviceName"`
}

type IdentityResponse struct {
	Certificate  string `json:"certificate"`
	PrivateKey   string `json:"privateKey"`
	IssuingCa    string `json:"issuingCa"`
	SerialNumber string `json:"serialNumber"`
}

func fetchIdentity(controlPlaneURL string, token utils.ServiceAccountToken, serviceName string) (tls.Certificate, error) {
	reqBody, err := json.Marshal(IdentityRequest{ServiceName: serviceName})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to marshal request: %v\n", err)
	}

	cpURL, err := url.Parse(controlPlaneURL)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to parse control plane URL: %v\n", err)
	}

	var headers http.Header = make(http.Header, 2)
	headers.Add("Content-Type", "application/json")
	headers.Add("X-Sentinel-Token", "Bearer "+string(token))

	req := http.Request{
		Method: http.MethodPost,
		URL:    cpURL,
		Header: headers.Clone(),
		Body:   io.NopCloser(bytes.NewBuffer(reqBody)),
	}
	var client http.Client
	resp, err := client.Do(&req)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to issue identity: %v\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tls.Certificate{}, fmt.Errorf("Unexpected response status: %d\n", resp.StatusCode)
	}

	var idResponse IdentityResponse
	if err := json.NewDecoder(resp.Body).Decode(&idResponse); err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to decode response: %v\n", err)
	}

	return tls.X509KeyPair([]byte(idResponse.Certificate), []byte(idResponse.PrivateKey))

}

func createProxy(targetURL *url.URL, l *slog.Logger) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		l.ErrorContext(r.Context(), "proxy_upstream_error",
			slog.String("error", err.Error()),
			slog.String("backend_url", targetURL.String()),
			slog.String("client_ip", r.RemoteAddr),
		)
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
	}

	return proxy
}

func coreHandlers(proxy *httputil.ReverseProxy, l *slog.Logger) *http.ServeMux {
	mainHandler := http.NewServeMux()
	mainHandler.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	proxyHandler := logger.Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
				clientName := r.TLS.PeerCertificates[0].Subject.CommonName
				l.InfoContext(r.Context(), "mTLS request allowed",
					slog.String("client_cn", clientName),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path))
			}
			proxy.ServeHTTP(w, r)
		}),
		l,
	)

	mainHandler.Handle("/", proxyHandler)

	return mainHandler
}

type Proxy struct {
	cfg         config.ProxyConfig
	certManager *certmanager.CertManager
	caCertPool  *x509.CertPool
	logger      *slog.Logger
	ctx         context.Context
	stopChan    chan struct{}
}

func NewProxy(ctx context.Context, cfg config.ProxyConfig, fetcher grpc.CertFetcher, tokenProvider utils.TokenProvider, logger *slog.Logger) (*Proxy, error) {
	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Root CA: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	clientset, err := utils.CreateClientset()
	if err != nil {
		return nil, err
	}
	token, err := tokenProvider.RequestToken(clientset, cfg.KubernetesNamespace, cfg.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service account token: %w", err)
	}
	tokenContainer := &utils.ServiceAccountTokenContainer{
		Token: token,
	}
	claims, err := utils.ParseJWT(string(tokenContainer.Token))
	if err != nil {
		return nil, err
	}
	spiffeId := claims.GetSpiffeId(cfg.TrustedDomain)
	// initialCert, err := fetchIdentity(cfg.ControlPlaneURL, token, "proxy")
	result, err := fetcher.Fetch(ctx, tokenContainer.Token, spiffeId)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch initial certificate: %w", err)
	}
	cm := &certmanager.CertManager{CurrentCert: &result.Certificate, CurrentPodUid: result.PodUid, RetryDelay: 1 * time.Minute, RenewalWindow: 5 * time.Minute}
	go cm.StartRotation(ctx, fetcher, tokenProvider, tokenContainer, cfg.KubernetesNamespace, cfg.ServiceName, cfg.TargetGRPC, clientset, logger)
	proxy := &Proxy{
		cfg:         cfg,
		logger:      logger,
		caCertPool:  caCertPool,
		ctx:         ctx,
		certManager: cm,
		stopChan:    make(chan struct{}),
	}
	return proxy, nil
}

func (p *Proxy) Run() error {
	if p.cfg.ProxyPort == 15006 || os.Getenv("PROXY_MODE") == "redirect" {
		return p.runRedirectMode()
	}

	return p.runDirectMode()
}

func (p *Proxy) runDirectMode() error {
	targetURL, _ := url.Parse(p.cfg.TargetURL)
	proxyHandler := createProxyHandler(targetURL, p.logger)

	tlsConfig := p.getInboundTLSConfig()
	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", p.cfg.ProxyPort),
		Handler:   proxyHandler,
		TLSConfig: tlsConfig,
	}

	p.logger.Info("starting proxy in direct mode", slog.Int("port", p.cfg.ProxyPort))
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			p.logger.Error("server error", slog.String("error", err.Error()))
		}
	}()
	<-stop
	p.logger.Info("shutting down proxy")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func (p *Proxy) runRedirectMode() error {
	podIP := os.Getenv("POD_IP")
	if podIP == "" {
		p.logger.Warn("POD_IP not set, cannot determine inbound/outbound")
	}
	tlsConfig := p.getInboundTLSConfig()
	ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", p.cfg.InboundPort), tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on rediracted port %d: %w", p.cfg.InboundPort, err)
	}
	p.logger.Info("starting proxy in redirect mode", slog.Int("port", p.cfg.InboundPort), slog.String("pod_ip", podIP))
	for {
		conn, err := ln.Accept()
		if err != nil {
			p.logger.Warn("accept error", slog.String("error", err.Error()))
			continue
		}
		p.handleRedirectedConnection(conn, podIP)
	}
}

func (p *Proxy) getInboundTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return p.certManager.GetCurrentCertificate(), nil
		},
		ClientCAs:  p.caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}
}
func (p *Proxy) getOutboundTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return p.certManager.GetCurrentCertificate(), nil
		},
		RootCAs:    p.caCertPool,
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
	}
}

func createProxyHandler(targetURL *url.URL, l *slog.Logger) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		l.ErrorContext(r.Context(), "proxy_upstream_error",
			slog.String("error", err.Error()),
			slog.String("backend_url", targetURL.String()),
			slog.String("client_ip", r.RemoteAddr))
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			w.WriteHeader(http.StatusUnauthorized)
		} else {
			w.WriteHeader(http.StatusBadGateway)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.Handle("/", logger.Middleware(proxy, l))
	return mux
}

func (p *Proxy) handleRedirectedConnection(conn net.Conn, podIP string) {
	defer conn.Close()
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		p.logger.Warn("not a TCP connection")
		return
	}
	originalDst, err := utils.GetOriginalDest(tcpConn)
	if err != nil {
		p.logger.Warn("failed to get original destination", slog.String("error", err.Error()))
		return
	}
	p.logger.Debug("redirected connection", slog.String("original_dst", originalDst.String()), slog.String("pod_ip", podIP))
	if podIP != "" && originalDst.String() == podIP {
		p.forwardLocalApp(conn)
	} else {
		p.forwardOutbound(conn, originalDst)
	}
}

func (p *Proxy) forwardLocalApp(clientConn net.Conn) {
	u, err := url.Parse(p.cfg.TargetURL)
	if err != nil {
		p.logger.Error("failed to parse target URL", slog.String("error", err.Error()))
		return
	}
	appConn, err := net.Dial("tcp", u.Host)
	if err != nil {
		p.logger.Error("failed to connect to app", slog.String("error", err.Error()), slog.String("target", u.Host))
		return
	}
	defer appConn.Close()
	p.proxyCopy(appConn, clientConn, u.Host)
}

func (p *Proxy) forwardOutbound(clientConn net.Conn, dst net.Addr) {
	tlsConn, ok := clientConn.(*tls.Conn)
	if !ok {
		p.logger.Error("inbound connection is not TLS", slog.String("destination", dst.String()))
		clientConn.Close()
		return
	}
	connState := tlsConn.ConnectionState()
	if len(connState.PeerCertificates) == 0 {
		p.logger.Error("no client certificate provided", slog.String("dst", dst.String()))
		clientConn.Close()
		return
	}

	clientCert := connState.PeerCertificates[0]
	clientIdentity := extractIdentity(clientCert)
	p.logger.Info("outbound request", slog.String("client_identity", clientIdentity), slog.String("destination", dst.String()))
	outboundTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*p.certManager.GetCurrentCertificate()},
		RootCAs:      p.caCertPool,
		ServerName:   extractServerName(dst.String()),
		MinVersion:   tls.VersionTLS13,
	}
	dialAddr := dst.String()
	outboundConn, err := tls.Dial("tcp", dialAddr, outboundTLSConfig)
	if err != nil {
		p.logger.Error("faiuled to establish outbound mTLS", slog.String("destination", dialAddr), slog.String("error", err.Error()))
		clientConn.Close()
		return
	}
	defer outboundConn.Close()
	p.proxyCopy(clientConn, outboundConn, dst.String())
}

func (p *Proxy) proxyCopy(dst, src net.Conn, name string) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, err := io.Copy(dst, src)
		if err != nil || err != io.EOF {
			p.logger.Warn("copy error",
				slog.String("direction", "src→dst"),
				slog.String("target", name),
				slog.Int64("bytes", n),
				slog.String("error", err.Error()))
		}
		dst.Close()
	}()
	go func() {
		defer wg.Done()
		n, err := io.Copy(src, dst)
		if err != nil || err != io.EOF {
			p.logger.Warn("copy error",
				slog.String("direction", "dst→src"),
				slog.String("target", name),
				slog.Int64("bytes", n),
				slog.String("error", err.Error()))
		}
		src.Close()
	}()
	wg.Wait()
}

func extractServerName(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func extractIdentity(cert *x509.Certificate) string {
	for _, san := range cert.DNSNames {
		if strings.HasPrefix(san, "spiffe://") {
			return san
		}
	}
	return cert.Subject.CommonName
}
