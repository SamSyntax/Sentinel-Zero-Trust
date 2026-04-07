package certmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"sentinel-zt/data-plane/internal/grpc"
	"sentinel-zt/data-plane/internal/utils"
	"sync"
	"time"
)

type CertManager struct {
	CertMutex     sync.RWMutex
	CurrentCert   *tls.Certificate
	CurrentPodUid string
	RetryDelay    time.Duration
	RenewalWindow time.Duration
	RenewNow      chan struct{}
}

func (cm *CertManager) GetCurrentCertificate() *tls.Certificate {
	cm.CertMutex.RLock()
	defer cm.CertMutex.RUnlock()
	return cm.CurrentCert
}

func (cm *CertManager) StartRotation(ctx context.Context, fetcher grpc.CertFetcher, tokenProvider utils.TokenProvider, token *utils.ServiceAccountTokenContainer, namespace, serviceName, spiffeId, target string, l *slog.Logger) {
	for {
		if token == nil || token.Mu == nil || token.Token == "" {
			l.ErrorContext(ctx, "can't start rotation", slog.String("error", "no service account token"), slog.String("service", serviceName))
			select {
			case <-time.After(cm.RetryDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		cm.CertMutex.RLock()
		needsFetch := cm.CurrentCert == nil || cm.CurrentCert.Certificate == nil
		var certDER []byte
		if !needsFetch {
			certDER = cm.CurrentCert.Certificate[0]
		}
		cm.CertMutex.RUnlock()

		if needsFetch {
			result, err := fetcher.Fetch(ctx, token.Token, spiffeId)
			if err != nil {
				l.WarnContext(ctx, "initial certificate fetch failed, retrying", slog.String("error", err.Error()), slog.String("service", serviceName))
				select {
				case <-time.After(cm.RetryDelay):
					continue
				case <-ctx.Done():
					return
				}
			}
			cm.CertMutex.Lock()
			cm.CurrentCert = &result.Certificate
			cm.CurrentPodUid = result.PodUid
			cm.CertMutex.Unlock()
			l.InfoContext(ctx, "initial certificate fetched", slog.String("service", serviceName), slog.String("podUid", result.PodUid))
			continue
		}

		leaf, err := x509.ParseCertificate(certDER)
		if err != nil {
			l.WarnContext(ctx, "failed to parse certificate, will re-fetch", slog.String("error", err.Error()), slog.String("service", serviceName))
			select {
			case <-time.After(cm.RetryDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		window := cm.RenewalWindow
		if window == 0 {
			window = 5 * time.Minute
		}
		renewTime := leaf.NotAfter.Add(-window)
		sleepDuration := time.Until(renewTime)
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}

		l.DebugContext(ctx, "certificate rotation scheduled",
			slog.String("service", serviceName),
			slog.Time("renewal_time", renewTime),
			slog.Duration("sleep_duration", sleepDuration))

		select {
		case <-time.After(sleepDuration):
		case <-cm.RenewNow:
		case <-ctx.Done():
			return
		}

		newToken, err := tokenProvider.RequestToken(namespace, serviceName)
		if err != nil {
			l.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName))
			select {
			case <-time.After(cm.RetryDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		token.Mu.Lock()
		token.Token = newToken
		token.Mu.Unlock()

		result, err := fetcher.Fetch(ctx, token.Token, spiffeId)
		if err != nil {
			l.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName))
			select {
			case <-time.After(cm.RetryDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		cm.CertMutex.Lock()
		cm.CurrentCert = &result.Certificate
		cm.CurrentPodUid = result.PodUid
		cm.CertMutex.Unlock()
		l.InfoContext(ctx, "certificate rotated successfully", slog.String("service", serviceName), slog.String("podUid", result.PodUid))
	}
}
