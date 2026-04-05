package proxy_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sentinel-zt/data-plane/internal/config"
	"sentinel-zt/data-plane/internal/grpc"
	"sentinel-zt/data-plane/internal/proxy"
	utils "sentinel-zt/data-plane/internal/utils"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const PRIVATE_KEY = `
-----BEGIN PRIVATE KEY-----
MIGEAgEAMBAGByqGSM49AgEGBSuBBAAKBG0wawIBAQQgAwSZHJPHsWYla2hFRKyG
7M1h2rMU08Nabqy40fiAPjWhRANCAATd/XVAPN0UyoDuTpDyKC/qPcPHbwC7P0rR
6a2A5zyaK0b3Pnuel4f1gwswRhxsDaekiz0QwQOHQ01QpPjQwa6/
-----END PRIVATE KEY-----
`

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func makeCert(t *testing.T, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}

func makeCACert(t *testing.T) (caCert *x509.Certificate, caKey *ecdsa.PrivateKey, caPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
	return ca, key, pemBytes
}

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "ca-*.crt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		t.Fatalf("write to temp file: %v", err)
	}
	return f.Name()
}

func generateTestJWT(t *testing.T, namespace, serviceAccount string) utils.ServiceAccountToken {
	t.Helper()
	claims := utils.KubernetesClaims{}
	claims.Kubernetes.Namespace = namespace
	claims.Kubernetes.ServiceAccount.Name = serviceAccount
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(
		jwt.SigningMethodES256,
		claims,
	)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tokenString, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return utils.ServiceAccountToken(tokenString)
}

type mockCertFetcher struct {
	mu        sync.Mutex
	callCount int
	results   []grpc.IdentityResult
	errors    []error
}

func (m *mockCertFetcher) Fetch(ctx context.Context, token utils.ServiceAccountToken, serviceName string) (grpc.IdentityResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callCount
	m.callCount++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return grpc.IdentityResult{}, m.errors[idx]
	}
	if idx < len(m.results) {
		return m.results[idx], nil
	}
	return grpc.IdentityResult{}, nil
}

func (m *mockCertFetcher) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type mockTokenProvider struct {
	mu        sync.Mutex
	callCount int
	tokens    []utils.ServiceAccountToken
	errors    []error
}

func (m *mockTokenProvider) RequestToken(namespace, serviceAccount string) (utils.ServiceAccountToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := m.callCount
	m.callCount++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return "", m.errors[idx]
	}
	if idx < len(m.tokens) {
		return m.tokens[idx], nil
	}
	return "mock-token", nil
}

func (m *mockTokenProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func generateMockSpiffeID(t *testing.T, serviceName string) string {
	t.Helper()
	return "spiffe://cluster.local/ns/default/sa/" + serviceName
}

func TestExtractServerName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.0.0.1:8080", "10.0.0.1"},
		{"10.0.0.1", "10.0.0.1"},
		{"example.com:443", "example.com"},
		{"[::1]:8080", "::1"},
		{"localhost:3000", "localhost"},
		{"localhost:8080", "localhost"},
		{"localhost", "localhost"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			actual := proxy.ExtractServerName(tc.input)
			if actual != tc.expected {
				t.Errorf("ExtractServerName(%q) = %q, got %q", tc.input, tc.expected, actual)
			}
		})
	}
}

func TextExtractIdentity(t *testing.T) {
	tests := []struct {
		name     string
		cert     *x509.Certificate
		expected string
	}{
		{
			name: "SPIFFE ID in DNSNames",
			cert: &x509.Certificate{
				DNSNames: []string{"example.com", "spiffe://cluster.local/ns/default/sa/test-svc"},
			},
			expected: "spiffe://cluster.local/ns/default/sa/test-svc",
		},
		{
			name: "no SPIFFE ID falls back to CN",
			cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "no-spiffe-service"},
				DNSNames: []string{"example.com"},
			},
			expected: "no-spiffe-service",
		},
		{
			name: "empty DNSNames falls back to CN",
			cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "fallback"},
				DNSNames: []string{},
			},
			expected: "fallback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := proxy.ExtractIdentity(tc.cert)
			if actual != tc.expected {
				t.Errorf("ExtractIdentity(%q) = %q, got %q", tc.name, tc.expected, actual)
			}
		})
	}
}

func newTestConfig(t *testing.T, caPath string) config.ProxyConfig {
	t.Helper()
	// token := generateTestJWT(t, "default", "test-sa")
	return config.ProxyConfig{
		ProxyPort:           0,
		ServiceName:         "test-sa",
		KubernetesNamespace: "default",
		CACertPath:          caPath,
		TrustedDomain:       "cluster.local",
		TargetURL:           "http://localhost:9090",
	}
}

func TestNewProxy_Success(t *testing.T) {
	_, _, caPEM := makeCACert(t)
	caPath := writeTempFile(t, caPEM)
	defer os.Remove(caPath)
	cert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{{Certificate: cert, PodUid: "test-pod-uid"}},
	}
	provider := &mockTokenProvider{
		tokens: []utils.ServiceAccountToken{generateTestJWT(t, "default", "test-sa")},
	}
	cfg := newTestConfig(t, caPath)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	p, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	if p == nil {
		t.Fatalf("NewProxy() returned a nil pointer")
	}

	if fetcher.CallCount() != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.CallCount())
	}
	if provider.CallCount() != 1 {
		t.Errorf("expected 1 token request, got %d", provider.CallCount())
	}
}

func TestNewProxy_FailsWithMissingCA(t *testing.T) {
	fetcher := &mockCertFetcher{}
	provider := &mockTokenProvider{}
	cfg := config.ProxyConfig{
		CACertPath: "/nope/ca.crt",
	}
	ctx := t.Context()
	_, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

func TestNewProxy_FailsWithBadJWT(t *testing.T) {
	_, _, caPEM := makeCACert(t)
	caPath := writeTempFile(t, caPEM)
	defer os.Remove(caPath)
	fetcher := &mockCertFetcher{}
	provider := &mockTokenProvider{
		tokens: []utils.ServiceAccountToken{"not-a-jwt"},
	}
	cfg := newTestConfig(t, caPath)
	ctx := t.Context()
	_, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err == nil {
		t.Error("expected error for invalid JWT")
	}
}

func TestNewProxy_FailsWhenFetchFails(t *testing.T) {
	_, _, caPEM := makeCACert(t)
	caPath := writeTempFile(t, caPEM)
	defer os.Remove(caPath)
	fetcher := &mockCertFetcher{
		errors: []error{errors.New("fetch failed")},
	}
	provider := &mockTokenProvider{
		tokens: []utils.ServiceAccountToken{generateTestJWT(t, "default", "test-sa")},
	}

	cfg := newTestConfig(t, caPath)
	ctx := t.Context()
	_, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err == nil {
		t.Error("expected error for fetch failure")
	}
	if !strings.Contains(err.Error(), "failed to fetch initial certificate") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetInboundTLSConfig(t *testing.T) {
	cert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{{Certificate: cert, PodUid: "test-pod-uid"}},
	}
	provider := &mockTokenProvider{
		tokens: []utils.ServiceAccountToken{generateTestJWT(t, "default", "test-sa")},
	}

	_, _, caPEM := makeCACert(t)
	caPath := writeTempFile(t, caPEM)
	defer os.Remove(caPath)

	cfg := newTestConfig(t, caPath)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}

	tlsConfig := p.GetInboundTLSConfig()
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert")
	}
	if tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS13, got %v", tlsConfig.MinVersion)
	}
	hello := &tls.ClientHelloInfo{}
	gotCert, err := tlsConfig.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate() error = %v", err)
	}
	if gotCert == nil {
		t.Fatalf("GetCertificate() returned nil certificate")
	}
}

func TestHealthzEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux := proxy.CreateProxyHandler(nil, noopLogger())
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got %q", rec.Body.String())
	}
}

func TestProxyHandler_ForwardRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "backend response")
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	mux := proxy.CreateProxyHandler(backendURL, noopLogger())

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Body.String() != "backend response" {
		t.Errorf("expected 'backend response', got %q", rec.Body.String())
	}
}

func TestProxyHandler_PreservesTraceHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	mux := proxy.CreateProxyHandler(backendURL, noopLogger())
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	req.Header.Set("X-Trace-ID", "test-trace-id")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("X-Trace-ID") != "test-trace-id" {
		t.Errorf("expected trace id %q, got %q", "test-trace-id", rec.Header().Get("X-Trace-ID"))
	}
}

func TestMTLS_ClientWithValidCert(t *testing.T) {
	ca, caKey, caPEM := makeCACert(t)
	caPath := writeTempFile(t, caPEM)
	defer os.Remove(caPath)
	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"spiffe://cluster.local/ns/default/sa/test-client"},
	}
	clientDER, _ := x509.CreateCertificate(rand.Reader, clientTmpl, ca, &clientKey.PublicKey, caKey)
	clientCert := tls.Certificate{
		Certificate: [][]byte{clientDER},
		PrivateKey:  clientKey,
	}

	serverCert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{{Certificate: serverCert, PodUid: "test-pod-uid"}},
	}
	provider := &mockTokenProvider{
		tokens: []utils.ServiceAccountToken{generateTestJWT(t, "default", "test-sa")},
	}
	cfg := newTestConfig(t, caPath)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p, err := proxy.NewProxy(ctx, cfg, fetcher, provider, noopLogger())
	if err != nil {
		t.Fatalf("NewProxy() error = %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	tlsConfig := p.GetInboundTLSConfig()
	tlsLn := tls.NewListener(ln, tlsConfig)
	defer tlsLn.Close()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "backend response")
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)
	handler := proxy.CreateProxyHandler(backendURL, noopLogger())
	go http.Serve(tlsLn, handler)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates:       []tls.Certificate{clientCert},
				RootCAs:            x509.NewCertPool(),
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Get(fmt.Sprintf("https://%s/", ln.Addr().String()))
	if err != nil {
		t.Fatalf("client request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}
