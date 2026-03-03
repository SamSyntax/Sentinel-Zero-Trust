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
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sentinel-zt/data-plane/internal/config"
	"sentinel-zt/data-plane/internal/logger"
	utils "sentinel-zt/data-plane/internal/utils"
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

type CertManager struct {
	certMutex   sync.RWMutex
	currentCert *tls.Certificate
}

func (cm *CertManager) StartRotation(ctx context.Context, token utils.ServiceAccountToken, serviceName string, controlPlaneURL string, l *slog.Logger) {
	for {
		if cm.currentCert.Certificate == nil {
			cert, err := fetchIdentity(controlPlaneURL, token, serviceName)
			if err != nil {
				l.WarnContext(ctx, "initial certificate fetch failed, retrying", slog.String("error", err.Error()), slog.String("service", serviceName))
				time.Sleep(time.Minute * 1)
				continue
			}
			cm.currentCert = &cert
			l.InfoContext(ctx, "initial certificate fetched", slog.String("service", serviceName))
		}
		cm.certMutex.RLock()
		leaf, err := x509.ParseCertificate(cm.currentCert.Certificate[0])
		cm.certMutex.RUnlock()
		if err != nil {
			l.WarnContext(ctx, "failed to parse certificate, will re-fetch", slog.String("error", err.Error()), slog.String("service", serviceName))
			time.Sleep(time.Minute * 1)
			continue
		}
		renewTime := leaf.NotAfter.Add(-5 * time.Minute)
		sleepDuration := time.Until(renewTime)
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}

		l.DebugContext(ctx, "certificate rotation scheduled",
			slog.String("service", serviceName),
			slog.Time("renewal_time", renewTime),
			slog.Duration("sleep_duration", sleepDuration))
		select {
		case <-time.After(sleepDuration):
			start := time.Now()
			newCert, err := fetchIdentity(controlPlaneURL, token, serviceName)
			duration := time.Since(start)
			if err != nil {
				l.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName), slog.Duration("duration", duration))
				time.Sleep(time.Minute * 1)
				continue
			} else {
				l.InfoContext(ctx, "certificate rotated successfully", slog.String("service", serviceName), slog.Duration("duration", duration))
			}
			cm.certMutex.Lock()
			cm.currentCert = &newCert
			cm.certMutex.Unlock()
		case <-ctx.Done():
			return
		}
	}
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

func Run(ctx context.Context, cfg config.ProxyConfig, l *slog.Logger) {
	token, err := utils.GetServiceAccountToken(l)
	if err != nil {
		l.ErrorContext(ctx, "Shutting down sentinel-zt proxy...", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.SetDefault(l)
	w := slog.NewLogLogger(l.Handler(), slog.LevelError)
	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		l.ErrorContext(ctx, "failed to read Root CA", slog.String("error", err.Error()))
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	initialCert, err := fetchIdentity(cfg.ControlPlaneURL, token, "proxy")
	if err != nil {
		l.ErrorContext(ctx, "failed to fetch initial certificate", slog.String("error", err.Error()))
	}
	cm := CertManager{currentCert: &initialCert}
	go cm.StartRotation(ctx, token, "proxy", cfg.ControlPlaneURL, l)

	targetURL, _ := url.Parse(cfg.TargetURL)
	proxy := createProxy(targetURL, l)
	mainHandler := coreHandlers(proxy, l)
	server := &http.Server{
		Addr:     fmt.Sprintf(":%d", cfg.ProxyPort),
		Handler:  mainHandler,
		ErrorLog: w,
		TLSConfig: &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cm.certMutex.RLock()
				defer cm.certMutex.RUnlock()
				return cm.currentCert, nil
			},
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
		},
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		l.InfoContext(ctx, "sentinel zt-proxy starting", slog.String("addr", server.Addr))
		listener, err := tls.Listen("tcp", server.Addr, server.TLSConfig)
		if err != nil {
			l.ErrorContext(ctx, "failed to create TLS listener", slog.String("error", err.Error()))
			return
		}
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			l.ErrorContext(ctx, "failed to serve", slog.String("error", err.Error()))
		}
	}()

	go utils.TestService(l)
	<-stop
	l.InfoContext(ctx, "Shutting down sentinel-zt proxy...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		l.ErrorContext(ctx, "Server forced to shutdown", slog.String("error", err.Error()))
	}
	l.InfoContext(ctx, "Sentinel-zt proxy exited")

}
