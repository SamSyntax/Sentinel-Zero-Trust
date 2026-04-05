package utils_test

import (
	"sentinel-zt/data-plane/internal/utils"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestJWT(t *testing.T, namespace, serviceAccount string) string {
	t.Helper()
	claims := utils.KubernetesClaims{}
	claims.Kubernetes.Namespace = namespace
	claims.Kubernetes.ServiceAccount.Name = serviceAccount

	token := jwt.NewWithClaims(
		jwt.SigningMethodNone,
		claims,
	)

	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return tokenString
}

type TestCase struct {
	Name               string
	TrustedDomain      string
	Namespace          string
	ServiceAccountName string
	ExpectedSpiffeId   string
	Success            bool
}

func TestJWTUtils_GetSpiffeId(t *testing.T) {
	var testCases = []TestCase{
		{
			Name:               "test-1",
			TrustedDomain:      "cluster.local",
			Namespace:          "default",
			ServiceAccountName: "test-svc",
			ExpectedSpiffeId:   "spiffe://cluster.local/ns/default/sa/test-svc",
			Success:            true,
		},
		{
			Name:               "test-2",
			TrustedDomain:      "cluster.local",
			Namespace:          "test-ns",
			ServiceAccountName: "test-svc-2",
			ExpectedSpiffeId:   "spiffe://cluster.local/ns/test-ns/sa/test-svc-2",
			Success:            true,
		},
		{
			Name:               "test-3",
			TrustedDomain:      "cluster.local",
			Namespace:          "test-3",
			ServiceAccountName: "test-svc-3",
			ExpectedSpiffeId:   "spiffe://cluster.local/ns/test-3/sa/test-svc-3",
			Success:            true,
		},
		{
			Name:               "missing domain",
			TrustedDomain:      "",
			Namespace:          "test-ns",
			ServiceAccountName: "test-svc-2",
			ExpectedSpiffeId:   "",
			Success:            false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			token := generateTestJWT(t, tc.Namespace, tc.ServiceAccountName)
			claims, err := utils.ParseJWT(token)
			if err != nil {
				t.Fatalf("failed to parse token: %v", err)
			}
			spiffeId, err := claims.GetSpiffeId(tc.TrustedDomain)
			if err != nil && tc.Success {
				t.Fatalf("Expected success but got error: %v", err)
			}
			if spiffeId != tc.ExpectedSpiffeId {
				t.Errorf("expected spiffe id %q, got %q", tc.ExpectedSpiffeId, spiffeId)
			}
		})
	}
}

func TestJWTUtils_ParseJWT(t *testing.T) {
	var testCases = []TestCase{
		{
			Name:               "success-1",
			TrustedDomain:      "cluster.local",
			Namespace:          "default",
			ServiceAccountName: "test-svc",
			ExpectedSpiffeId:   "spiffe://cluster.local/ns/default/sa/test-svc",
			Success:            true,
		},
		{
			Name:               "success-2",
			TrustedDomain:      "cluster.local",
			Namespace:          "test-ns",
			ServiceAccountName: "test-svc-2",
			ExpectedSpiffeId:   "spiffe://cluster.local/ns/test-ns/sa/test-svc-2",
			Success:            true,
		},
		{
			Name:               "missing namespace",
			TrustedDomain:      "cluster.local",
			Namespace:          "",
			ServiceAccountName: "test-svc-3",
			ExpectedSpiffeId:   "",
			Success:            false,
		},
		{
			Name:               "missing service account",
			TrustedDomain:      "cluster.local",
			Namespace:          "test-ns",
			ServiceAccountName: "",
			ExpectedSpiffeId:   "",
			Success:            false,
		},
		{
			Name:               "missing domain",
			TrustedDomain:      "",
			Namespace:          "test-ns",
			ServiceAccountName: "test-svc",
			ExpectedSpiffeId:   "",
			Success:            false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			token := generateTestJWT(t, tc.Namespace, tc.ServiceAccountName)
			claims, err := utils.ParseJWT(token)
			if err != nil {
				t.Fatalf("failed to parse token: %v", err)
			}

			if claims.Kubernetes.Namespace != tc.Namespace {
				t.Errorf("expected namespace %q, got %q", tc.Namespace, claims.Kubernetes.Namespace)
			}
			if claims.Kubernetes.ServiceAccount.Name != tc.ServiceAccountName {
				t.Errorf("expected service account %q, got %q", tc.ServiceAccountName, claims.Kubernetes.ServiceAccount.Name)
			}
		})
	}
}
