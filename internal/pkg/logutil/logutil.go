package logutil

import (
	"log/slog"
	"os"
)

func NewLogger(groupName string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug, AddSource: true,
	})).WithGroup(groupName)
}
