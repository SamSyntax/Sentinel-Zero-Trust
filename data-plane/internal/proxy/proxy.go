package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
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

func startCertificateRotation(serviceName string, ctx context.Context, logger *slog.Logger) {
	for {
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

		logger.InfoContext(ctx, "rotation scheduled", slog.String("service", serviceName), slog.String("renewal_time", renewTime.Local().Format(time.ANSIC)))
		time.Sleep(sleepDuration)
		logger.InfoContext(ctx, "rotating certificate", slog.String("service", serviceName))
		newCert, err := fetchIdentity(serviceName)
		if err != nil {
			slog.ErrorContext(ctx, "rotation failed", slog.String("error", err.Error()), slog.String("service", serviceName))
			continue
		}
		certMutex.Lock()
		currentCert = newCert
		certMutex.Unlock()
	}
}

func fetchIdentity(serviceName string) (tls.Certificate, error) {
	reqBody, err := json.Marshal(IdentityRequest{ServiceName: serviceName})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("Failed to marshal request: %v\n", err)
	}

	resp, err := http.Post("http://localhost:8081/api/v1/identity/issue", "application/json", bytes.NewBuffer(reqBody))
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.ErrorContext(ctx, "failed to get user home dir", slog.String("error", err.Error()))
	}
	certDirPath := fmt.Sprintf("%s/Documents/FinalProject/sentinel-zt/certs/", homeDir)
	caCert, err := os.ReadFile(certDirPath + "root_ca.crt")
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

	tlsConfig := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			certMutex.RLock()
			defer certMutex.RUnlock()
			return &currentCert, nil
		},
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}

	targetURL, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	mTLSHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			clientName := r.TLS.PeerCertificates[0].Subject.CommonName
			logger.InfoContext(ctx, "allow request", slog.String("service", clientName))
		}
		proxy.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    ":8443",
		Handler: mTLSHandler,
	}
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("Target application reached successfully.\n"))
		})
		http.ListenAndServe(":8080", mux)
	}()

	log.Println("Sentinel ZT-Proxy listening on :8443 (mTLS enforced)")
	listener, err := tls.Listen("tcp", server.Addr, tlsConfig)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create TLS listener", slog.String("error", err.Error()))
	}
	slog.InfoContext(ctx, "sentinel zt-proxy listening", slog.String("address", server.Addr))
	err = server.Serve(listener)
	if err != nil {
		logger.ErrorContext(ctx, "failed to serve", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
