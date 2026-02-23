package main

import (
	"context"
	"fmt"
	"os"
	"sentinel-zt/data-plane/internal/logger"
	"sentinel-zt/data-plane/internal/proxy"
)

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "local"
	}
	logFile, err := os.OpenFile(fmt.Sprintf("infra/logs/proxy/%s.log", env), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	l, cleanup := logger.InitLogger(logger.Config{
		Level:       "Info",
		IsJSON:      true,
		AddSource:   false,
		AppVersion:  "0.0.1",
		ServiceName: "Control Plane",
		Env:         env,
	}, logFile, 5000)
	defer cleanup()
	ctx := context.WithValue(context.Background(), "trace_id", "tx_999")

	proxy.Run(ctx, l, logFile)
}
