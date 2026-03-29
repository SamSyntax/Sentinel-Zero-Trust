package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sentinel-zt/data-plane/internal/utils"
	"sentinel-zt/data-plane/proto"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type IdentityResult struct {
	Certificate tls.Certificate
	PodUid      string
}

func FetchIdentityGRPC(ctx context.Context, target string, token utils.ServiceAccountToken, serviceName string) (IdentityResult, error) {
	md := metadata.Pairs(
		"x-sentinel-token", fmt.Sprintf("Bearer %s", strings.TrimSpace(string(token))),
	)
	reqCtx := metadata.NewOutgoingContext(ctx, md)
	conn, _ := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := proto.NewCertificateIssuerServiceClient(conn)
	resp, err := client.GetCertificate(reqCtx, &proto.CertificateRequest{
		ServiceName: serviceName,
	})
	if err != nil {
		return IdentityResult{}, err
	}

	cert, err := tls.X509KeyPair([]byte(resp.GetCertificate()), []byte(resp.GetPrivateKey()))
	if err != nil {
		return IdentityResult{}, err
	}

	return IdentityResult{
		Certificate: cert,
		PodUid:      resp.GetPodUid(),
	}, nil
}
