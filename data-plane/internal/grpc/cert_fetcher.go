package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sentinel-zt/data-plane/internal/utils"
	"sentinel-zt/data-plane/proto"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type IdentityResult struct {
	Certificate tls.Certificate
	PodUid      string
}

type RealCertFetcher struct {
	GrpcClient *grpc.ClientConn
}

type CertFetcher interface {
	Fetch(ctx context.Context, token utils.ServiceAccountToken, spiffeId string) (IdentityResult, error)
}

func (cf *RealCertFetcher) Fetch(ctx context.Context, token utils.ServiceAccountToken, spiffeId string) (IdentityResult, error) {
	md := metadata.Pairs(
		"x-sentinel-token", fmt.Sprintf("Bearer %s", strings.TrimSpace(string(token))),
	)
	reqCtx := metadata.NewOutgoingContext(ctx, md)

	/*
	 * This kills the connection after we return, so retry logic can't be used
	 *
	 *	defer cf.GrpcClient.Close()
	 *
	 * */

	client := proto.NewCertificateIssuerServiceClient(cf.GrpcClient)
	resp, err := client.GetCertificate(reqCtx, &proto.CertificateRequest{
		ServiceName: spiffeId,
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
