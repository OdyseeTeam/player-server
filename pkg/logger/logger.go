package logger

import (
	"io"
	"log/slog"
	"os"
	"sync/atomic"
)

const (
	EnvTest = "test"
	EnvProd = "prod"
)

var (
	defaultLevel = new(slog.LevelVar)
	defaultLogger atomic.Pointer[slog.Logger]
)

func init() {
	defaultLevel.Set(slog.LevelInfo)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: defaultLevel,
	})
	logger := slog.New(handler)
	defaultLogger.Store(logger)
	slog.SetDefault(logger)
}

func ConfigureDefaults(level slog.Level) {
	defaultLevel.Set(level)
}

func GetLogger() *slog.Logger {
	return defaultLogger.Load()
}

func WithComponent(component string) *slog.Logger {
	return defaultLogger.Load().With("component", component)
}

func DisableLogger() {
	handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError + 1,
	})
	logger := slog.New(handler)
	defaultLogger.Store(logger)
}

func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	Flush()
	os.Exit(1)
}
