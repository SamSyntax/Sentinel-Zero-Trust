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

func (cm *CertManager) StartRotation(ctx context.Context, fetcher grpc.CertFetcher, tokenProvider utils.TokenProvider, token *utils.ServiceAccountTokenContainer, namespace string, serviceName string, target string, l *slog.Logger) {
	for {
		cm.CertMutex.Lock()
		if token == nil || token.Mu == nil || token.Token == "" {
			l.ErrorContext(ctx, "can't start rotation", slog.String("error", "no service account token"), slog.String("service", serviceName))
			time.Sleep(cm.RetryDelay)
			cm.CertMutex.Unlock()
			continue
		}
		if cm.CurrentCert == nil || cm.CurrentCert.Certificate == nil {
			result, err := fetcher.Fetch(ctx, token.Token, serviceName)
			if err != nil {
				l.WarnContext(ctx, "initial certificate fetch failed, retrying", slog.String("error", err.Error()), slog.String("service", serviceName))
				time.Sleep(cm.RetryDelay)
				cm.CertMutex.Unlock()
				continue
			}
			cm.CurrentCert = &result.Certificate
			cm.CurrentPodUid = result.PodUid
			l.InfoContext(ctx, "initial certificate fetched", slog.String("service", serviceName), slog.String("podUid", result.PodUid))
			cm.CertMutex.Unlock()
		}
		cm.CertMutex.RLock()
		leaf, err := x509.ParseCertificate(cm.CurrentCert.Certificate[0])
		cm.CertMutex.RUnlock()
		if err != nil {
			l.WarnContext(ctx, "failed to parse certificate, will re-fetch", slog.String("error", err.Error()), slog.String("service", serviceName))
			time.Sleep(cm.RetryDelay)
			cm.CertMutex.Unlock()
			continue
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
			cm.CertMutex.Unlock()
			return
		}
		start := time.Now()
		newToken, err := tokenProvider.RequestToken(namespace, serviceName)
		duration := time.Since(start)
		if err != nil {
			l.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName), slog.Duration("duration", duration))
			time.Sleep(cm.RetryDelay)
			cm.CertMutex.Unlock()
			continue
		}
		token.Mu.Lock()
		token.Token = newToken
		token.Mu.Unlock()
		result, err := fetcher.Fetch(ctx, token.Token, serviceName)
		duration = time.Since(start)
		if err != nil {
			l.WarnContext(ctx, "certificate rotation failed, will retry", slog.String("error", err.Error()), slog.String("service", serviceName), slog.Duration("duration", duration))
			time.Sleep(cm.RetryDelay)
			cm.CertMutex.Unlock()
			continue
		} else {
			l.InfoContext(ctx, "certificate rotated successfully", slog.String("service", serviceName), slog.String("podUid", result.PodUid), slog.Duration("duration", duration))
		}
		cm.CertMutex.Unlock()
	}
}
