package grpc

import (
	"context"
	"sentinel-zt/data-plane/internal/utils"
)

type CertFetcher interface {
	Fetch(ctx context.Context, token utils.ServiceAccountToken, serviceName string) (IdentityResult, error)
}
