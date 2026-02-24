package main

import (
	"context"
	"os"
	"sentinel-zt/data-plane/internal/logger"
	"sentinel-zt/data-plane/internal/proxy"
)

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
		ServiceName: "Control Plane",
		Env:         env,
	}, os.Stdout, 5000)
	defer cleanup()
	ctx := context.WithValue(context.Background(), "trace_id", "tx_999")

	proxy.Run(ctx, l)
}
