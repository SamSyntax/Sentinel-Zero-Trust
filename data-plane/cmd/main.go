package main

import (
	"context"
	"log/slog"
	"os"
	"sentinel-zt/data-plane/internal/config"
	sgrpc "sentinel-zt/data-plane/internal/grpc"
	"sentinel-zt/data-plane/internal/logger"
	"sentinel-zt/data-plane/internal/proxy"
	"sentinel-zt/data-plane/internal/utils"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	env := os.Getenv("ENV")

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "Debug"
	}
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "sentinel-data-plane"

	}

	ctx := context.WithValue(context.Background(), "APP_NAME", serviceName)
	var loggerCfg config.LoggerConfig = config.LoggerConfig{
		Level:       logLevel,
		IsJSON:      true,
		AddSource:   false,
		AppVersion:  "0.0.1",
		ServiceName: serviceName,
		Env:         env,
		Context:     ctx,
	}
	logger, cleanup := logger.InitLogger(loggerCfg, os.Stdout, 5000)
	defer cleanup()

	proxyMode := os.Getenv("PROXY_MODE")
	var port int
	if proxyMode == "redirect" {
		port = 15006
	} else {
		port = 8443
	}
	var cfg config.ProxyConfig
	if env == "" {
		env = "local"
		loggerCfg.Env = env
		cfg = config.CreateProxyConfig(8444, serviceName, loggerCfg)
	} else {
		cfg = config.CreateProxyConfig(8443, serviceName, loggerCfg)
	}
	cfg.Load()

	conn, err := grpc.NewClient(cfg.TargetGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("failed to connect to target", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	fetcher := &sgrpc.RealCertFetcher{
		GrpcClient: conn,
	}
	clientset, err := utils.CreateClientset()
	if err != nil {
		logger.Error("failed to create clientset", slog.String("error", err.Error()))
		os.Exit(1)
	}
	tokenProvider := &utils.RealTokenProvider{
		Clientset: clientset,
	}

	logger.Info("starting sentinel-zt proxy", slog.String("mode", proxyMode), slog.Int("port", port))
	p, err := proxy.NewProxy(ctx, cfg, fetcher, tokenProvider, logger)
	if err != nil {
		logger.Error("failed to create proxy", slog.String("error", err.Error()))
		for {
			logger.Info("Retrying to create proxy in 10 seconds...")
			time.Sleep(10 * time.Second)
			p, err = proxy.NewProxy(ctx, cfg, fetcher, tokenProvider, logger)
			if err == nil {
				break
			}
			logger.Error("failed to create proxy", slog.String("error", err.Error()))
		}
	}
	if err := p.Run(); err != nil {
		logger.Error("proxy error", slog.String("error", err.Error()))
		os.Exit(1)
	}

}
