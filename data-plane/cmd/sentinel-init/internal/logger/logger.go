package logger

import (
	"context"
	"io"
	"log/slog"
	"sentinel-init/internal/config"
)

const (
	TraceIDKey        = "trace_id"
	RequestIDKey      = "request_id"
	HttpMethodKey     = "http_method"
	HttpUriKey        = "http_uri"
	HttpRemoteAddrKey = "http_remote_addr"
	ErrorKey          = "error"
)

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if val, ok := ctx.Value(TraceIDKey).(string); ok {
		r.AddAttrs(slog.String(TraceIDKey, val))
	}
	if val, ok := ctx.Value(RequestIDKey).(string); ok {
		r.AddAttrs(slog.String(RequestIDKey, val))
	}
	if val, ok := ctx.Value(HttpMethodKey).(string); ok {
		r.AddAttrs(slog.String(HttpMethodKey, val))
	}
	if val, ok := ctx.Value(HttpUriKey).(string); ok {
		r.AddAttrs(slog.String(HttpUriKey, val))
	}
	if val, ok := ctx.Value(HttpRemoteAddrKey).(string); ok {
		r.AddAttrs(slog.String(HttpRemoteAddrKey, val))
	}

	return h.Handler.Handle(ctx, r)
}


func InitLogger(cfg config.LoggerConfig, out io.Writer, queueSize int) (*slog.Logger, func()) {
	asyncWriter := NewAsyncWriter(out, queueSize)
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}
	var baseHandler slog.Handler

	if cfg.IsJSON {
		baseHandler = slog.NewJSONHandler(asyncWriter, opts)
	} else {
		baseHandler = slog.NewTextHandler(asyncWriter, opts)
	}
	handler := &ContextHandler{baseHandler}
	logger := slog.New(handler).With(
		slog.String("version", cfg.AppVersion),
		slog.String("env", cfg.Env),
		slog.String("APP_NAME", cfg.ServiceName),
	)
	return logger, func() { asyncWriter.Close() }
}
