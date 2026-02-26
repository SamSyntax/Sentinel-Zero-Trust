package main

import (
	"context"
	"log/slog"
	"os"
	"sentinel-zt/data-plane/internal/logger"
	"sentinel-zt/data-plane/internal/proxy"
)

var GlobalLogger *slog.Logger

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "local"
	}
	l, cleanup := logger.InitLogger(logger.Config{
		Level:       "Info",
		IsJSON:      true,
		AddSource:   false,
		AppVersion:  "0.0.1",
		ServiceName: "Data Plane",
		Env:         env,
	}, os.Stdout, 5000)
	GlobalLogger = l
	defer cleanup()
	ctx := context.WithValue(context.Background(), "trace_id", "tx_999")

	proxy.Run(ctx, l)
}
