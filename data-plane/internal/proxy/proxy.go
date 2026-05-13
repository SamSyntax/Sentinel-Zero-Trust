package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
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

func CoreHandlers(proxy *httputil.ReverseProxy, l *slog.Logger) *http.ServeMux {
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

	serviceAccount := cfg.ServiceAccount
	if serviceAccount == "" {
		serviceAccount = cfg.ServiceName
	}

	token, err := tokenProvider.RequestToken(cfg.KubernetesNamespace, serviceAccount, cfg.PodName, cfg.PodUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service account token: %w", err)
	}
	tokenContainer := &utils.ServiceAccountTokenContainer{
		Token: token,
		Mu:    &sync.RWMutex{},
	}
	claims, err := utils.ParseJWT(string(tokenContainer.Token))
	if err != nil {
		return nil, err
	}
	spiffeId, err := claims.GetSpiffeId(cfg.TrustedDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to get spiffe id: %w", err)
	}
	result, err := fetcher.Fetch(ctx, tokenContainer.Token, spiffeId)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch initial certificate: %w", err)
	}
	cm := &certmanager.CertManager{
		CurrentCert:   &result.Certificate,
		CurrentPodUid: result.PodUid,
		RetryDelay:    1 * time.Minute,
		RenewalWindow: 5 * time.Minute,
		RenewNow:      make(chan struct{}),
	}
	go cm.StartRotation(ctx, fetcher, tokenProvider, tokenContainer, cfg.KubernetesNamespace, cfg.ServiceName, serviceAccount, cfg.PodName, cfg.PodUID, spiffeId, cfg.TargetGRPC, logger)
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

	if os.Getenv("PROXY_MODE") == "tproxy" {
		return p.runTProxyMode()
	}

	return p.runDirectMode()
}

func (p *Proxy) runDirectMode() error {
	targetURL, _ := url.Parse(p.cfg.TargetURL)
	proxyHandler := CreateProxyHandler(targetURL, p.logger)

	tlsConfig := p.GetInboundTLSConfig()
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

	tlsConfig := p.GetInboundTLSConfig()
	inboundLn, err := tls.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p.cfg.InboundPort), tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on redirected port %d: %w", p.cfg.InboundPort, err)
	}
	outboundLn, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", p.cfg.OutboundPort))
	if err != nil {
		inboundLn.Close()
		return fmt.Errorf("failed to listen on outbound redirected port %d: %w", p.cfg.OutboundPort, err)
	}
	p.logger.Info("starting proxy in redirect mode",
		slog.Int("inbound_port", p.cfg.InboundPort),
		slog.Int("outbound_port", p.cfg.OutboundPort),
		slog.String("pod_ip", podIP))

	errCh := make(chan error, 2)
	go p.acceptRedirectInboundLoop(inboundLn, podIP, errCh)
	go p.acceptOutboundLoop(outboundLn, podIP, errCh)

	return <-errCh
}

func (p *Proxy) acceptRedirectInboundLoop(ln net.Listener, podIP string, errCh chan<- error) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- fmt.Errorf("inbound accept error: %w", err)
			return
		}
		p.logger.Info("accepted inbound redirected connection", slog.String("remote_addr", conn.RemoteAddr().String()))
		go p.handleRedirectedConnection(conn, podIP)
	}
}

func (p *Proxy) acceptOutboundLoop(ln net.Listener, podIP string, errCh chan<- error) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- fmt.Errorf("outbound accept error: %w", err)
			return
		}
		p.logger.Info("accepted outbound redirected connection", slog.String("remote_addr", conn.RemoteAddr().String()))
		go p.handleOutboundConnection(conn, podIP)
	}
}

func (p *Proxy) GetInboundTLSConfig() *tls.Config {
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

func CreateProxyHandler(targetURL *url.URL, l *slog.Logger) http.Handler {
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
	p.logger.Info("forwarded inbound redirected connection", slog.String("remote_addr", conn.RemoteAddr().String()), slog.String("pod_ip", podIP))
	p.forwardLocalApp(conn)
}

func (p *Proxy) forwardLocalApp(clientConn net.Conn) {
	u, err := url.Parse(p.cfg.TargetURL)
	if err != nil {
		p.logger.Error("failed to parse target URL", slog.String("error", err.Error()))
		return
	}

	if tlsConn, ok := clientConn.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			p.logger.Error("TLS handshake failed while forwarding to local app", slog.String("error", err.Error()), slog.String("target", u.Host))
			return
		}
		appConn, err := net.Dial("tcp", u.Host)
		if err != nil {
			p.logger.Error("failed to connect to app", slog.String("error", err.Error()), slog.String("target", u.Host))
			return
		}
		defer appConn.Close()
		p.logger.Info("forwarding to local app (TLS terminated)", slog.String("target", u.Host))

		if err := p.proxyInboundHTTPRequests(tlsConn, appConn, u.Host); err != nil {
			p.logger.Error("inbound HTTP proxy failed", slog.String("target", u.Host), slog.String("error", err.Error()))
		}
	} else {
		appConn, err := net.Dial("tcp", u.Host)
		if err != nil {
			p.logger.Error("failed to connect to app", slog.String("error", err.Error()), slog.String("target", u.Host))
			return
		}
		defer appConn.Close()
		p.logger.Info("forwarding to local app", slog.String("target", u.Host))
		p.proxyCopy(appConn, clientConn, u.Host)
	}
}

func (p *Proxy) forwardOutbound(clientConn net.Conn, dst net.Addr) {
	tlsConn, ok := clientConn.(*tls.Conn)
	if !ok {
		p.logger.Error("inbound connection is not TLS", slog.String("destination", dst.String()))
		clientConn.Close()
		return
	}

	err := tlsConn.Handshake()
	if err != nil {
		p.logger.Error("TLS handshake failed", slog.String("destination", dst.String()), slog.String("error", err.Error()))
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
	clientIdentity := ExtractIdentity(clientCert)
	p.logger.Info("outbound request", slog.String("client_identity", clientIdentity), slog.String("destination", dst.String()))

	outboundTLSConfig := &tls.Config{
		Certificates:       []tls.Certificate{*p.certManager.GetCurrentCertificate()},
		RootCAs:            p.caCertPool,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
	dialAddr := dst.String()
	outboundConn, err := tls.Dial("tcp", dialAddr, outboundTLSConfig)
	if err != nil {
		p.logger.Error("failed to establish outbound mTLS", slog.String("destination", dialAddr), slog.String("error", err.Error()))
		clientConn.Close()
		return
	}
	defer outboundConn.Close()
	p.proxyCopy(clientConn, outboundConn, dst.String())
}

func (p *Proxy) handleOutboundConnection(clientConn net.Conn, podIP string) {
	defer clientConn.Close()

	tcpConn, ok := clientConn.(*net.TCPConn)
	if !ok {
		p.logger.Warn("outbound connection is not TCP")
		return
	}

	originalDst, err := utils.GetOriginalDest(tcpConn)
	if err != nil {
		p.logger.Warn("failed to get original destination for outbound connection", slog.String("error", err.Error()))
		return
	}
	p.logger.Info("resolved original destination for outbound connection", slog.String("original_dst", originalDst.String()))

	if podIP != "" {
		if host, _, splitErr := net.SplitHostPort(originalDst.String()); splitErr == nil && host == podIP {
			p.forwardLocalApp(clientConn)
			return
		}
	}

	p.forwardOutboundPlain(clientConn, originalDst)
}

func (p *Proxy) forwardOutboundPlain(clientConn net.Conn, dst net.Addr) {
	p.logger.Info("forwarding outbound (plaintext to mTLS)", slog.String("destination", dst.String()))

	outboundTLSConfig := &tls.Config{
		Certificates:       []tls.Certificate{*p.certManager.GetCurrentCertificate()},
		RootCAs:            p.caCertPool,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}

	outboundConn, err := tls.Dial("tcp", dst.String(), outboundTLSConfig)
	if err != nil {
		p.logger.Error("failed to establish outbound mTLS", slog.String("destination", dst.String()), slog.String("error", err.Error()))
		return
	}
	defer outboundConn.Close()

	if err := p.proxyOutboundHTTPRequests(clientConn, outboundConn, dst.String()); err != nil {
		p.logger.Error("outbound HTTP proxy failed", slog.String("destination", dst.String()), slog.String("error", err.Error()))
	}
}

func (p *Proxy) proxyInboundHTTPRequests(clientConn *tls.Conn, appConn net.Conn, target string) error {
	clientReader := bufio.NewReader(clientConn)
	appReader := bufio.NewReader(appConn)

	clientIdentity := "unknown"
	if state := clientConn.ConnectionState(); len(state.PeerCertificates) > 0 {
		clientIdentity = ExtractIdentity(state.PeerCertificates[0])
	}

	for {
		req, err := http.ReadRequest(clientReader)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			p.logger.DebugContext(p.cfg.LoggerConfig.Context, "inbound HTTP request (EOF)", slog.String("client_identity", clientIdentity))
			return nil
		}
		if err != nil {
			return err
		}

		p.logger.Info("inbound HTTP request",
			slog.String("client_identity", clientIdentity),
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.String("target", target),
		)

		req.RequestURI = ""
		req.Close = true
		if err := req.Write(appConn); err != nil {
			return err
		}

		resp, err := http.ReadResponse(appReader, req)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			p.logger.DebugContext(p.cfg.LoggerConfig.Context, "inbound HTTP request (EOF)", slog.String("client_identity", clientIdentity))
			return nil
		}
		if err != nil {
			return err
		}

		p.logger.Info("inbound HTTP response",
			slog.String("client_identity", clientIdentity),
			slog.Int("status", resp.StatusCode),
			slog.String("target", target),
		)

		resp.Close = true
		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		return nil
	}
}

func (p *Proxy) proxyOutboundHTTPRequests(clientConn net.Conn, outboundConn net.Conn, destination string) error {
	clientReader := bufio.NewReader(clientConn)
	outboundReader := bufio.NewReader(outboundConn)

	for {
		req, err := http.ReadRequest(clientReader)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			p.logger.DebugContext(p.cfg.LoggerConfig.Context, "inbound HTTP request (EOF)", slog.String("remote_addr", clientConn.RemoteAddr().String()))
			return nil
		}
		if err != nil {
			return err
		}

		p.logger.Info("outbound HTTP request",
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
			slog.String("host", req.Host),
			slog.String("destination", destination),
		)

		req.RequestURI = ""
		req.Close = true
		if err := req.Write(outboundConn); err != nil {
			return err
		}

		resp, err := http.ReadResponse(outboundReader, req)
		if err != nil {
			return err
		}

		p.logger.Info("outbound HTTP response",
			slog.Int("status", resp.StatusCode),
			slog.String("destination", destination),
		)

		resp.Close = true
		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		return nil
	}
}

func (p *Proxy) proxyCopy(dst, src net.Conn, name string) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, err := io.Copy(dst, src)
		if err != nil && err != io.EOF {
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
		if err != nil && err != io.EOF {
			p.logger.Warn("copy error",
				slog.String("direction", "dst→src"),
				slog.String("target", name),
				slog.Int64("bytes", n),
				slog.String("error", err.Error()))
		}
		src.Close()
	}()
	p.logger.Info("accepted outbound redirected connection", slog.String("name", name), slog.String("remote_addr", src.RemoteAddr().String()))
	wg.Wait()
}

func ExtractServerName(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func ExtractIdentity(cert *x509.Certificate) string {
	for _, san := range cert.DNSNames {
		if strings.HasPrefix(san, "spiffe://") {
			return san
		}
	}
	return cert.Subject.CommonName
}

func createTProxyListener(port int) (net.Listener, error) {
	lc := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
			})
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
}

func (p *Proxy) runTProxyMode() error {
	podIP := os.Getenv("POD_IP")
	if podIP == "" {
		p.logger.Warn("POD_IP not set, cannot determine inbound/outbound")
	}

	inboundLn, err := createTProxyListener(p.cfg.InboundPort)
	if err != nil {
		return fmt.Errorf("failed to create TPROXY listener on port %d: %w", p.cfg.InboundPort, err)
	}
	outboundLn, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p.cfg.OutboundPort))
	if err != nil {
		inboundLn.Close()
		return fmt.Errorf("failed to listen on outbound TPROXY port %d: %w", p.cfg.OutboundPort, err)
	}
	tlsConfig := p.GetInboundTLSConfig()
	tlsLn := tls.NewListener(inboundLn, tlsConfig)
	p.logger.Info("starting proxy in TPROXY mode",
		slog.Int("inbound_port", p.cfg.InboundPort),
		slog.Int("outbound_port", p.cfg.OutboundPort),
		slog.String("pod_ip", podIP))

	errCh := make(chan error, 2)
	go p.acceptTProxyInboundLoop(tlsLn, podIP, errCh)
	go p.acceptOutboundLoop(outboundLn, podIP, errCh)

	return <-errCh
}

func (p *Proxy) acceptTProxyInboundLoop(ln net.Listener, podIP string, errCh chan<- error) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- fmt.Errorf("inbound accept error: %w", err)
			return
		}
		p.logger.Debug("accepted inbound TPROXY connection", slog.String("remote_addr", conn.RemoteAddr().String()))
		go p.handleTProxyConnection(conn, podIP)
	}
}

func (p *Proxy) handleTProxyConnection(clientConn net.Conn, podIP string) {
	defer clientConn.Close()

	tlsConn, ok := clientConn.(*tls.Conn)
	if !ok {
		p.logger.Error("inbound connection is not TLS - rejected", slog.String("error", "connection is not TLS"))
		return
	}

	err := tlsConn.Handshake()
	if err != nil {
		p.logger.Error("TLS handshake failed - rejecting connection", slog.String("error", err.Error()))
		return
	}

	connState := tlsConn.ConnectionState()
	if len(connState.PeerCertificates) == 0 {
		p.logger.Error("no client certificate provided")
		return
	}

	clientCert := connState.PeerCertificates[0]
	clientIdentity := ExtractIdentity(clientCert)

	originalDst, err := getOriginalDst(clientConn)
	if err != nil {
		p.logger.Warn("failed to get original destination", slog.String("error", err.Error()))
		return
	}

	remoteAddr := clientConn.RemoteAddr().(*net.TCPAddr)
	p.logger.Info("TPROXY connection",
		slog.String("client_identity", clientIdentity),
		slog.String("client_ip", remoteAddr.IP.String()),
		slog.String("original_dst", originalDst.String()),
		slog.String("pod_ip", podIP),
	)

	if podIP != "" {
		if host, _, _ := net.SplitHostPort(originalDst.String()); host == podIP {
			p.forwardLocalAppTProxy(tlsConn, clientIdentity)
			return
		}
	}
	p.forwardOutboundTProxy(tlsConn, originalDst, clientIdentity)
}

func getOriginalDst(conn net.Conn) (net.Addr, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, fmt.Errorf("not a TCP connection")
	}
	return utils.GetOriginalDest(tcpConn)
}

func (p *Proxy) forwardLocalAppTProxy(clientConn *tls.Conn, clientIdentity string) {
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
	p.logger.Info("forwarding to local app (mTLS)",
		slog.String("target", u.Host),
		slog.String("client_identity", clientIdentity),
	)

	p.proxyCopy(appConn, clientConn, u.Host)
}

func (p *Proxy) forwardOutboundTProxy(clientConn *tls.Conn, dst net.Addr, clientIdentity string) {
	p.logger.Info("forwarding outbound (mTLS)",
		slog.String("client_identity", clientIdentity),
		slog.String("destination", dst.String()))

	outboundTLSConfig := &tls.Config{
		Certificates:       []tls.Certificate{*p.certManager.GetCurrentCertificate()},
		RootCAs:            p.caCertPool,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}

	dialAddr := dst.String()
	outboundConn, err := tls.Dial("tcp", dialAddr, outboundTLSConfig)
	if err != nil {
		p.logger.Error("failed to establish outbound mTLS",
			slog.String("destination", dialAddr),
			slog.String("error", err.Error()))
		return
	}
	defer outboundConn.Close()
	p.proxyCopy(clientConn, outboundConn, dst.String())
}
