package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
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

func startCertificateRotation(serviceName string) {
	for {
		certMutex.RLock()
		leaf, err := x509.ParseCertificate(currentCert.Certificate[0])
		certMutex.RUnlock()
		if err != nil {
			log.Printf("[Control Plane] <Rotation> Failed to parse certificate: %v\n", err)
			time.Sleep(time.Minute * 1)
			continue
		}
		renewTime := leaf.NotAfter.Add(-5 * time.Minute)
		sleepDuration := time.Until(renewTime)
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}
		log.Printf("[Control Plane] <Rotation> Next renewal scheduled in %v\n", renewTime.Local().Format(time.ANSIC))
		time.Sleep(sleepDuration)
		log.Printf("[Control Plane] <Rotation> Rotating certificate for service: %s\n", serviceName)
		newCert, err := fetchIdentity(serviceName)
		if err != nil {
			log.Printf("[Control Plane] <Rotation> Failed to fetch new certificate: %v\n", err)
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
		return tls.Certificate{}, fmt.Errorf("[Control Plane] Failed to marshal request: %v\n", err)
	}

	resp, err := http.Post("http://localhost:8081/api/v1/identity/issue", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("[Control Plane] Failed to issue identity: %v\n", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tls.Certificate{}, fmt.Errorf("[Control Plane] Unexpected response status: %d\n", resp.StatusCode)
	}

	var idResponse IdentityResponse
	if err := json.NewDecoder(resp.Body).Decode(&idResponse); err != nil {
		return tls.Certificate{}, fmt.Errorf("[Control Plane] Failed to decode response: %v\n", err)
	}

	log.Printf("[Control Plane] Received identity for service: %s\n", serviceName)
	return tls.X509KeyPair([]byte(idResponse.Certificate), []byte(idResponse.PrivateKey))

}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home dir: %v\n", err)
	}
	certDirPath := fmt.Sprintf("%s/Documents/FinalProject/sentinel-zt/certs/", homeDir)
	caCert, err := os.ReadFile(certDirPath + "root_ca.crt")
	if err != nil {
		log.Fatalf("Failed to read Root CA: %v\n", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	initialCert, err := fetchIdentity("proxy")
	if err != nil {
		log.Fatalf("%v", err)
	}

	currentCert = initialCert

	go startCertificateRotation("proxy")

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
			log.Printf("[Control Plane] <ALLOW> Request from authenticated service: %s\n", clientName)
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
