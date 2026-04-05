package grpc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sentinel-zt/data-plane/proto"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	sgrpc "sentinel-zt/data-plane/internal/grpc"
)

const bufSize = 1024 * 1024

var listener *bufconn.Listener

func init() {
	listener = bufconn.Listen(bufSize)
}

func bufDialer(context.Context, string) (net.Conn, error) {
	return listener.Dial()
}

type testCertServer struct {
	proto.UnimplementedCertificateIssuerServiceServer
	t *testing.T
}

func (s *testCertServer) GetCertificate(ctx context.Context, req *proto.CertificateRequest) (*proto.CertificateResponse, error) {
	cert, key := generateTestCert(s.t, req.GetServiceName())
	return &proto.CertificateResponse{
		Certificate: cert,
		PrivateKey:  key,
		PodUid:      "test-pod-uid",
	}, nil
}

func generateTestCert(t *testing.T, serviceName string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serviceName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{serviceName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return string(certPEM), string(keyPEM)
}

func startTestServer(t *testing.T) *grpc.Server {
	t.Helper()
	s := grpc.NewServer()
	proto.RegisterCertificateIssuerServiceServer(s, &testCertServer{t: t})
	go func() {
		if err := s.Serve(listener); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	t.Cleanup(s.Stop)
	return s
}

func generateMockSpiffeID(t *testing.T, serviceName string) string {
	t.Helper()
	return "spiffe://cluster.local/ns/default/sa/" + serviceName
}

func TestRealCertFetcher_FetchSuccess(t *testing.T) {
	startTestServer(t)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()
	fetcher := &sgrpc.RealCertFetcher{GrpcClient: conn}
	result, err := fetcher.Fetch(context.Background(), "token", generateMockSpiffeID(t, "test-svc"))
	if err != nil {
		t.Fatalf("fetch returned error: %v", err)
	}
	t.Logf("result: %v", result)
	if result.PodUid != "test-pod-uid" {
		t.Errorf("expected pod uid %q, got %q", "test-pod-uid", result.PodUid)
	}
	if result.Certificate.Certificate == nil {
		t.Error("expected certificate to be set")
	}
}

func TestRealCertFetcher_FetchWithEmptyToken(t *testing.T) {
	startTestServer(t)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	fetcher := &sgrpc.RealCertFetcher{GrpcClient: conn}
	_, err = fetcher.Fetch(context.Background(), "", generateMockSpiffeID(t, "test-svc"))
	if err != nil {
		t.Logf("fetch with empty token returned error (expected in real scenario): %v", err)
	}

}
