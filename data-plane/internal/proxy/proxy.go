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
	"sentinel-zt/data-plane/internal/config"
	"sentinel-zt/data-plane/internal/logger"
	gl "sentinel-zt/data-plane/internal/logger"
	"sync"
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

var (
	certMutex   sync.RWMutex
	currentCert *tls.Certificate
)

type ServiceAccountToken string

var ServiceAccountTokenValue ServiceAccountToken

func GetServiceAccountToken() {
	file, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		gl.GlobalLogger.WarnContext(context.Background(), "failed to get pod service account token, will retry", slog.String("error", err.Error()))
		return
	}
	ServiceAccountTokenValue = ServiceAccountToken(file)
	gl.GlobalLogger.InfoContext(context.Background(), "service account token loaded successfully")
}

func startCertificateRotation(serviceName string, ctx context.Context) {
	for {
		if currentCert.Certificate == nil {
			cert, err := fetchIdentity(serviceName)
			if err != nil {
				gl.GlobalLogger.WarnContext(ctx, "initial certificate fetch failed, retrying", slog.String("error", err.Error()), slog.String("service", serviceName))
				time.Sleep(time.Minute * 1)
				continue
			}
			currentCert = &cert
			gl.GlobalLogger.InfoContext(ctx, "initial certificate fetched", slog.String("service", serviceName))
		}
		certMutex.RLock()
		leaf, err := x509.ParseCertificate(currentCert.Certificate[0])
		certMutex.RUnlock()
		if err != nil {
			gl.GlobalLogger.WarnContext(ctx, "failed to parse certificate, will re-fetch", slog.String("error", err.Error()), slog.String("service", serviceName))
			time.Sleep(time.Minute * 1)
			continue
		}
		renewTime := leaf.NotAfter.Add(-5 * time.Minute)
		sleepDuration := time.Until(renewTime)
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}

		gl.GlobalLogger.DebugContext(ctx, "certificate rotation scheduled",
			slog.String("service", serviceName),
			slog.Time("renewal_time", renewTime),
			slog.Duration("sleep_duration", sleepDuration))
		select {
		case <-time.After(sleepDuration):
			start := time.Now()
			newCert, err := fetchIdentity(serviceName)
			duration := time.Since(start)
			if err != nil {
				gl.GlobalLogger.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName), slog.Duration("duration", duration))
				time.Sleep(time.Minute * 1)
				continue
			} else {
				gl.GlobalLogger.InfoContext(ctx, "certificate rotated successfully", slog.String("service", serviceName), slog.Duration("duration", duration))
			}
			certMutex.Lock()
			currentCert = &newCert
			certMutex.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func fetchIdentity(serviceName string) (tls.Certificate, error) {
	reqBody, err := json.Marshal(IdentityRequest{ServiceName: serviceName})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to marshal request: %v\n", err)
	}

	cpURLstr := os.Getenv("CONTROL_PLANE_URL")
	if cpURLstr == "" {
		cpURLstr = "http://localhost:8081/api/v1/identity/issue"
	}
	cpURL, err := url.Parse(cpURLstr)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to parse control plane URL: %v\n", err)
	}

	var headers http.Header = make(http.Header, 2)
	headers.Add("Content-Type", "application/json")
	headers.Add("X-Sentinel-Token", "Bearer "+string(ServiceAccountTokenValue))

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

func Run(ctx context.Context) {
	GetServiceAccountToken()
	slog.SetDefault(gl.GlobalLogger)
	w := slog.NewLogLogger(gl.GlobalLogger.Handler(), slog.LevelError)
	caPath := os.Getenv("CA_CERT_PATH")
	if caPath == "" {
		caPath = "../certs/root_ca.crt"
	}
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		gl.GlobalLogger.ErrorContext(ctx, "failed to read Root CA", slog.String("error", err.Error()))
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	initialCert, err := fetchIdentity("proxy")
	if err != nil {
		gl.GlobalLogger.ErrorContext(ctx, "failed to fetch initial certificate", slog.String("error", err.Error()))
	}

	currentCert = &initialCert

	go startCertificateRotation("proxy", ctx)

	targetStr := os.Getenv("TARGET_URL")
	if targetStr == "" {
		targetStr = "http://localhost:8080"
	}
	targetURL, _ := url.Parse(targetStr)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		gl.GlobalLogger.ErrorContext(r.Context(), "proxy_upstream_error",
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
	loggingHandler := logger.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			clientName := r.TLS.PeerCertificates[0].Subject.CommonName
			gl.GlobalLogger.InfoContext(r.Context(), "mTLS request allowed",
				slog.String("client_cn", clientName),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
		}
		proxy.ServeHTTP(w, r)
	}))
	server := &http.Server{
		Addr:     fmt.Sprintf(":%d", config.GlobalConfig.ProxyPort),
		Handler:  loggingHandler,
		ErrorLog: w,
		TLSConfig: &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				certMutex.RLock()
				defer certMutex.RUnlock()
				return currentCert, nil
			},
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
		},
	}
	proxy.ErrorLog = w
	server.ErrorLog = w

	go testService()
	gl.GlobalLogger.InfoContext(ctx, "sentinel zt-proxy starting", slog.String("addr", server.Addr))

	listener, err := tls.Listen("tcp", server.Addr, server.TLSConfig)
	if err != nil {
		gl.GlobalLogger.ErrorContext(ctx, "failed to create TLS listener", slog.String("error", err.Error()))
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		gl.GlobalLogger.ErrorContext(ctx, "failed to serve", slog.String("error", err.Error()))
	}
}

func getHostAddress() string {
	var ip net.IP
	ifaces, _ := net.Interfaces()
	for _, a := range ifaces {
		addrs, _ := a.Addrs()
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				ip = net.IPv4(byte('1'), byte('2'), byte('3'), byte('4'))
			}
		}
	}
	return ip.To4().String()
}

func testService() {
	mux := http.NewServeMux()
	ip := getHostAddress()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Target application %s reached successfully.\n", ip)
	})
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		gl.GlobalLogger.ErrorContext(context.Background(), "failed to serve", slog.String("error", err.Error()))
	}
}
