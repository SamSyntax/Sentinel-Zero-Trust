package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
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
	currentCert tls.Certificate
)

type ServiceAccountToken string

var ServiceAccountTokenValue ServiceAccountToken

func GetServiceAccountToken() {
	file, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		gl.GlobalLogger.ErrorContext(context.WithValue(context.Background(), "trace_id", "tx_999"), "failed to get pod service account token", slog.String("error", err.Error()), slog.String("caller", "getServiceAccountToken()"))
		os.Exit(1)
	}
	ServiceAccountTokenValue = ServiceAccountToken(file)
}

func startCertificateRotation(serviceName string, ctx context.Context, logger *slog.Logger) {
	for {
		if currentCert.Certificate == nil {
			cert, err := fetchIdentity(serviceName)
			logger.ErrorContext(ctx, "initial cert rotation failed", slog.String("error", err.Error()), slog.String("service", serviceName))
			currentCert = cert
			time.Sleep(time.Minute * 1)
			continue
		}
		certMutex.RLock()
		leaf, err := x509.ParseCertificate(currentCert.Certificate[0])
		certMutex.RUnlock()
		if err != nil {
			logger.ErrorContext(ctx, "rotation failed", slog.String("error", err.Error()), slog.String("service", serviceName))
			time.Sleep(time.Minute * 1)
			continue
		}
		renewTime := leaf.NotAfter.Add(-5 * time.Minute)
		sleepDuration := time.Until(renewTime)
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}

		logger.InfoContext(ctx, "rotation scheduled",
			slog.String("service", serviceName),
			slog.Time("renewal_time", renewTime))
		select {
		case <-time.After(sleepDuration):
			start := time.Now()
			newCert, err := fetchIdentity(serviceName)
			duration := time.Since(start)
			logger.InfoContext(ctx, "rotating certificate", slog.String("service", serviceName), slog.Duration("duration", duration))
			if err != nil {
				logger.ErrorContext(ctx, "rotation failed", slog.String("error", err.Error()), slog.String("service", serviceName))
				continue
			}
			certMutex.Lock()
			currentCert = newCert
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

	cpURL := os.Getenv("CONTROL_PLANE_URL")
	if cpURL == "" {
		cpURL = "http://localhost:8081/api/v1/identity/issue"
	}

	resp, err := http.Post(cpURL, "application/json", bytes.NewBuffer(reqBody))
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

func Run(ctx context.Context, logger *slog.Logger) {
	GetServiceAccountToken()
	slog.SetDefault(logger)
	w := slog.NewLogLogger(logger.Handler(), slog.LevelError)
	caPath := os.Getenv("CA_CERT_PATH")
	if caPath == "" {
		caPath = "../certs/root_ca.crt"
	}
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		logger.ErrorContext(ctx, "failed to read Root CA", slog.String("error", err.Error()))
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	initialCert, err := fetchIdentity("proxy")
	if err != nil {
		logger.ErrorContext(ctx, "failed to fetch initial certificate", slog.String("error", err.Error()))
	}

	currentCert = initialCert

	go startCertificateRotation("proxy", ctx, logger)

	targetStr := os.Getenv("TARGET_URL")
	if targetStr == "" {
		targetStr = "http://localhost:8080"
	}
	targetURL, _ := url.Parse(targetStr)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.ErrorContext(r.Context(), "proxy_upstream_error",
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
	mTLSHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			clientName := r.TLS.PeerCertificates[0].Subject.CommonName
			logger.InfoContext(r.Context(), "mTLS request allowed",
				slog.String("client_cn", clientName),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
		}
		proxy.ServeHTTP(w, r)

	})
	server := &http.Server{
		Addr:     ":8443",
		Handler:  mTLSHandler,
		ErrorLog: w,
		TLSConfig: &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				certMutex.RLock()
				defer certMutex.RUnlock()
				return &currentCert, nil
			},
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
		},
	}
	proxy.ErrorLog = w
	server.ErrorLog = w

	go testService(logger)
	logger.InfoContext(ctx, "sentinel zt-proxy starting", slog.String("addr", server.Addr))

	listener, err := tls.Listen("tcp", server.Addr, server.TLSConfig)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create TLS listener", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		logger.ErrorContext(ctx, "failed to serve", slog.String("error", err.Error()))
		os.Exit(1)
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

func testService(l *slog.Logger) {
	mux := http.NewServeMux()
	ip := getHostAddress()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Target application %s reached successfully.\n", ip)
	})
	err := http.ListenAndServe(":8080", mux)
	if err != nil {
		l.ErrorContext(context.Background(), "failed to serve", slog.String("error", err.Error()))
	}
}
