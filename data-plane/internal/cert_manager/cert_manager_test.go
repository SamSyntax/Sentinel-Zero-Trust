package certmanager_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"log/slog"
	"math/big"
	"os"
	certmanager "sentinel-zt/data-plane/internal/cert_manager"
	"sentinel-zt/data-plane/internal/grpc"
	"sentinel-zt/data-plane/internal/utils"
	"sync"
	"testing"
	"time"
)

type mockCertFetcher struct {
	mu        sync.Mutex
	callCount int
	results   []grpc.IdentityResult
	errors    []error
}

const serviceName = "test-svc"
const spiffeId = "spiffe://cluster.local/ns/default/sa/" + serviceName

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

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestGetCurrentCertificate_NilInitially(t *testing.T) {
	cm := &certmanager.CertManager{}
	if cm.GetCurrentCertificate() != nil {
		t.Error("expected nil certificate initally")
	}
}

func TestGetCurrentCertificate_ConcurrentAccess(t *testing.T) {
	cert := makeCert(t, time.Now().Add(time.Hour))
	cm := certmanager.CertManager{CurrentCert: &cert, RetryDelay: 10 * time.Millisecond}
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			cm.GetCurrentCertificate()
		})
		wg.Go(func() {
			cm.CertMutex.Lock()
			cm.CurrentPodUid = "updated"
			cm.CertMutex.Unlock()
		})
	}
	wg.Wait()
}

func TestStartRotation_InitialFetchSuccess(t *testing.T) {
	cert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{{
			Certificate: cert,
			PodUid:      "test-pod-uid",
		}},
	}
	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{Token: "initial-token", Mu: &sync.RWMutex{}}
	ctx, cancel := context.WithCancel(context.Background())

	cm := &certmanager.CertManager{
		RetryDelay: 10 * time.Millisecond,
	}
	go cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	time.Sleep(200 * time.Millisecond)
	if cm.GetCurrentCertificate() == nil {
		t.Error("expected certificate to be set after initial fetch")
	}
	if cm.CurrentPodUid != "test-pod-uid" {
		t.Errorf("expected pod uid %q, got %q", "test-pod-uid", cm.CurrentPodUid)
	}
	if fetcher.CallCount() != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.CallCount())
	}

	cancel()
}

func TestStartRotation_InitialFetchRetriesOnError(t *testing.T) {
	cert := makeCert(t, time.Now().Add(time.Minute))
	fetcher := &mockCertFetcher{
		errors: []error{errors.New("fetch failed"), nil},
		results: []grpc.IdentityResult{
			{},
			{Certificate: cert, PodUid: "pod-1"},
		},
	}

	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{
		Token: "token",
		Mu:    &sync.RWMutex{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		RetryDelay: 10 * time.Millisecond,
	}
	go cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	time.Sleep(61 * time.Millisecond)

	if cm.GetCurrentCertificate() == nil {
		t.Error("expected certificate after retry")
	}

	if fetcher.CallCount() != 2 {
		t.Errorf("expected 2 fetch calls (fail + retry), got %d", fetcher.CallCount())
	}

	cancel()
}

func TestStartRotation_RotateCertWhenNearExpiry(t *testing.T) {
	cert1 := makeCert(t, time.Now().Add(500*time.Millisecond))
	cert2 := makeCert(t, time.Now().Add(time.Hour))

	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{
			{Certificate: cert1, PodUid: "pod-1"},
			{Certificate: cert2, PodUid: "pod-2"},
		},
	}

	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{Token: "token", Mu: &sync.RWMutex{}}
	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		RetryDelay:    10 * time.Millisecond,
		RenewalWindow: 200 * time.Millisecond,
		RenewNow:      make(chan struct{}, 1),
	}
	go cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	time.Sleep(50 * time.Millisecond)
	cm.RenewNow <- struct{}{}
	time.Sleep(50 * time.Millisecond)

	if fetcher.CallCount() < 2 {
		t.Errorf("expected at least 2 fetch calls (initial + rotation), got %d", fetcher.CallCount())
	}
	if cm.CurrentPodUid != "pod-2" {
		t.Errorf("expected rotated pod uid %q, got %q", "pod-2", cm.CurrentPodUid)
	}

	cancel()
}

func TestStartRotation_StopsOnContextCancel(t *testing.T) {
	cert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{{PodUid: "pod-1", Certificate: cert}},
	}
	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{Token: "token", Mu: &sync.RWMutex{}}
	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		RetryDelay: 10 * time.Millisecond,
	}
	go cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(200 * time.Millisecond)
	initialCalls := fetcher.CallCount()
	time.Sleep(300 * time.Millisecond)
	if fetcher.CallCount() != initialCalls {
		t.Errorf("exepcted no more fetch calls after cancel, got %d -> %d", initialCalls, fetcher.CallCount())
	}
}

func TestStartRotation_NoPanicWhenCertAlreadySet(t *testing.T) {
	// This is the exact scenario that triggered the original deadlock/panic:
	// CurrentCert is already set when StartRotation enters the loop,
	// so the needsFetch branch is not taken and the code proceeds
	// to parse the existing certificate.

	cert := makeCert(t, time.Now().Add(time.Hour))
	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{
			{Certificate: cert, PodUid: "pod-rotated"},
		},
	}
	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{
		Token: "token",
		Mu:    &sync.RWMutex{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		CurrentCert:   &cert,
		CurrentPodUid: "pod-initial",
		RetryDelay:    10 * time.Millisecond,
		RenewalWindow: 5 * time.Minute,
		RenewNow:      make(chan struct{}, 1),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	}()

	// Give the goroutine time to enter the loop and reach the
	// certificate parsing path (the one that previously deadlocked).
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartRotation did not exit after cancel; likely deadlocked")
	}
}

func TestStartRotation_ConcurrentGetDuringRotation(t *testing.T) {
	// Hammers GetCurrentCertificate while StartRotation is actively
	// running to surface any lock-ordering issues.
	cert := makeCert(t, time.Now().Add(500*time.Millisecond))
	cert2 := makeCert(t, time.Now().Add(time.Hour))

	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{
			{Certificate: cert2, PodUid: "pod-2"},
			{Certificate: cert2, PodUid: "pod-3"},
			{Certificate: cert2, PodUid: "pod-4"},
		},
	}
	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{
		Token: "token",
		Mu:    &sync.RWMutex{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		CurrentCert:   &cert,
		CurrentPodUid: "pod-1",
		RetryDelay:    10 * time.Millisecond,
		RenewalWindow: 400 * time.Millisecond,
		RenewNow:      make(chan struct{}, 1),
	}

	go cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())

	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			for range 50 {
				c := cm.GetCurrentCertificate()
				if c == nil {
					t.Error("GetCurrentCertificate returned nil during rotation")
					return
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
	wg.Wait()
	cancel()
}

func TestStartRotation_RenewNowWithExistingCert(t *testing.T) {
	// Cert is pre-set and long-lived; Force an immediate renewal
	// via RenewNow. This exercises the scheduling + renewal path that
	// previously held a write lock while trying to acquire a read lock.
	cert1 := makeCert(t, time.Now().Add(time.Hour))
	cert2 := makeCert(t, time.Now().Add(2*time.Hour))

	fetcher := &mockCertFetcher{
		results: []grpc.IdentityResult{
			{Certificate: cert2, PodUid: "pod-renewed"},
		},
	}
	provider := &mockTokenProvider{}
	token := &utils.ServiceAccountTokenContainer{
		Token: "token",
		Mu:    &sync.RWMutex{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cm := &certmanager.CertManager{
		CurrentCert:   &cert1,
		CurrentPodUid: "pod-original",
		RetryDelay:    10 * time.Millisecond,
		RenewalWindow: 5 * time.Minute,
		RenewNow:      make(chan struct{}, 1),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cm.StartRotation(ctx, fetcher, provider, token, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	}()

	// Wait for the goroutine to reach the select/sleep, then poke it.
	time.Sleep(100 * time.Millisecond)
	cm.RenewNow <- struct{}{}
	time.Sleep(100 * time.Millisecond)

	if cm.CurrentPodUid != "pod-renewed" {
		t.Errorf("expected pod uid %q, got %q", "pod-renewed", cm.CurrentPodUid)
	}
	if fetcher.CallCount() != 1 {
		t.Errorf("expected 1 fetch call, got %d", fetcher.CallCount())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartRotation did not exit after cancel; likely deadlocked")
	}
}

func TestStartRotation_NilTokenContainerRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &mockCertFetcher{}
	provider := &mockTokenProvider{}

	cm := &certmanager.CertManager{
		RetryDelay: 10 * time.Millisecond,
		RenewNow:   make(chan struct{}, 1),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cm.StartRotation(ctx, fetcher, provider, nil, "default", serviceName, spiffeId, "localhost:9090", noopLogger())
	}()

	// Let it spin on the nil-token retry path a few times.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartRotation did not exit after cancel with nil token; likely deadlocked")
	}

	if fetcher.CallCount() != 0 {
		t.Errorf("expected 0 fetch calls with nil token, got %d", fetcher.CallCount())
	}
}
