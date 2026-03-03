package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

const (
	TraceIDHeader   = "X-Trace-ID"
	RequestIDHeader = "X-Request-ID"
)

type ctxKey string

const (
	ctxTraceIDKey        ctxKey = "trace_id"
	ctxRequestIDKey      ctxKey = "request_id"
	ctxHttpMethodKey     ctxKey = "http_method"
	ctxHttpUriKey        ctxKey = "http_uri"
	ctxHttpRemoteAddrKey ctxKey = "http_remote_addr"
	ctxErrorKey          ctxKey = "error"
)

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func Middleware(next http.Handler, l *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get(TraceIDHeader)
		if traceID == "" {
			traceID = generateTraceID()
		}

		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = generateTraceID()
		}

		ctx := WithTraceID(r.Context(), traceID)
		ctx = context.WithValue(ctx, ctxRequestIDKey, requestID)
		ctx = context.WithValue(ctx, ctxHttpMethodKey, r.Method)
		ctx = context.WithValue(ctx, ctxHttpUriKey, r.RequestURI)
		ctx = context.WithValue(ctx, ctxHttpRemoteAddrKey, r.RemoteAddr)

		w.Header().Set(TraceIDHeader, traceID)
		w.Header().Set(RequestIDHeader, requestID)

		startTime := time.Now()

		ww := &statusResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(ww, r.WithContext(ctx))

		duration := time.Since(startTime)

		level := slog.LevelInfo
		if ww.statusCode >= 500 {
			level = slog.LevelError
		} else if ww.statusCode >= 400 {
			level = slog.LevelWarn
		}

		l.Log(ctx, level, "http_request",
			slog.String("method", r.Method),
			slog.String("uri", r.RequestURI),
			slog.Int("status", ww.statusCode),
			slog.Duration("duration", duration),
			slog.String("client_ip", r.RemoteAddr),
		)
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
