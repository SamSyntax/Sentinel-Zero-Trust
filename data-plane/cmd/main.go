package main

import (
	"context"
	"os"
	"sentinel-zt/data-plane/internal/config"
	"sentinel-zt/data-plane/internal/logger"
	"sentinel-zt/data-plane/internal/proxy"
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
	if env == "" {
		env = "local"
		loggerCfg.Env = env
		config.GlobalConfig = config.CreateProxyConfig(8444, "proxy-local", loggerCfg)
	} else {
		config.GlobalConfig = config.CreateProxyConfig(8443, "proxy", loggerCfg)
	}
	_, cleanup := logger.InitLogger(loggerCfg, os.Stdout, 5000)
	defer cleanup()

	proxy.Run(ctx)
}
