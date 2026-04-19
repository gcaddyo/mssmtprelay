package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// New creates a text slog logger with level from config/env.
func New(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv})
	return slog.New(h)
}

// WithRequest enriches logger with Graph request correlation IDs when present.
func WithRequest(ctx context.Context, logger *slog.Logger, requestID, clientRequestID string) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	attrs := make([]any, 0, 4)
	if requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	if clientRequestID != "" {
		attrs = append(attrs, "client_request_id", clientRequestID)
	}
	if len(attrs) == 0 {
		return logger
	}
	return logger.With(attrs...)
}
