package utils_test

import (
	"context"
	"sentinel-zt/data-plane/internal/utils"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type mockTokenProvider struct {
	tokens    []utils.ServiceAccountToken
	errors    []error
	mu        sync.Mutex
	callCount int
}

func (m *mockTokenProvider) Fetch(ctx context.Context, token utils.ServiceAccountToken, serviceName string) (utils.ServiceAccountToken, error) {
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

func TestReatlTokenProvider_RequestToken(t *testing.T) {
	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		t.Skipf("envtest not available: %v", err)
	}
	defer env.Stop()
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	_, err = cs.CoreV1().ServiceAccounts("default").Create(
		context.Background(),
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "test-sa"}},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatalf("create SA: %v", err)
	}

	provider := utils.NewRealTokenProvider(cs)
	token, err := provider.RequestToken("default", "test-sa")
	if err != nil {
		t.Fatalf("request token: %v", err)
	}
	if token == "" {
		t.Error("expected token, got empty")
	}
}

func TestRealTokenProvider_RequestToken_NonExistenntSA(t *testing.T) {
	env := &envtest.Environment{}
	cfg, err := env.Start()

	if err != nil {
		t.Skipf("envtest not available: %v", err)
	}
	defer env.Stop()
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}
	provider := utils.NewRealTokenProvider(cs)
	_, err = provider.RequestToken("default", "non-existent-sa")
	if err == nil {
		t.Error("expected error for non-existent sa")
	}
}
