package contexutil

import (
	"context"
	"log/slog"
)

type contextKey struct{}

var loggerKey = contextKey{}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger.WithGroup("worker"))
}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok {
		return slog.Default()
	}
	return logger
}
