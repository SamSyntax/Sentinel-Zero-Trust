package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

type VaultBundle struct {
	Data struct {
		Certificate string `json:"certificate"`
		PrivateKey  string `json:"private_key"`
	} `json:"data"`
}

func loadIdentity(filePath string) (tls.Certificate, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return tls.Certificate{}, err
	}

	var bundle VaultBundle
	if err := json.Unmarshal(bytes, &bundle); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair([]byte(bundle.Data.Certificate), []byte(bundle.Data.PrivateKey))
}

func main() {
	caCert, err := os.ReadFile("../certs/root_ca.crt")
	if err != nil {
		log.Fatalf("Failed to read Root CA: %v", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	proxyCert, err := loadIdentity("../certs/proxy-bundle.json")
	if err != nil {
		log.Fatalf("Failed to load Proxy identity: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{proxyCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	targetURL, _ := url.Parse("http://localhost:8080")
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	mTLSHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			clientName := r.TLS.PeerCertificates[0].Subject.CommonName
			log.Printf("[ALLOW] Request from authenticated service: %s\n", clientName)
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
		log.Fatalf("Failed to create TLS listener: %v\n", err)
	}
	log.Fatal(server.Serve(listener))
}
