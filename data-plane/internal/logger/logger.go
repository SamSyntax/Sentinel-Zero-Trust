package logger

import (
	"context"
	"log/slog"
	"os"
)

const (
	TraceIDKey = "trace_id"
	ErrorKey   = "error"
)

type Config struct {
	Level       string
	IsJSON      bool
	AddSource   bool
	AppVersion  string
	ServiceName string
	Env         string
}

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

	return h.Handler.Handle(ctx, r)
}

func InitLogger(cfg Config, queueSize int) (*slog.Logger, func()) {
	asyncWriter := NewAsyncWriter(os.Stdout, queueSize)
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
		slog.String("service_name", cfg.ServiceName),
	)
	return logger, func() { asyncWriter.Close() }

}
