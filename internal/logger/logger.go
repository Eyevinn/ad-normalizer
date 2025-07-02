package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

var defaultLogger *slog.Logger
var errorLogger *slog.Logger

func init() {

	loglevelEnv, _ := os.LookupEnv("LOG_LEVEL")
	loglevelEnv = strings.ToUpper(loglevelEnv)

	loglevel := slog.LevelInfo

	switch loglevelEnv {
	case "DEBUG":
		loglevel = slog.LevelDebug
	case "WARN":
		loglevel = slog.LevelWarn
	case "ERROR":
		loglevel = slog.LevelError
	}

	defaultLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: loglevel,
	}))
	errorLogger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	}))

	slog.SetDefault(defaultLogger)
}

func Debug(msg string, attrs ...slog.Attr) {
	slog.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

func Info(msg string, attrs ...slog.Attr) {
	slog.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

func Warn(msg string, attrs ...slog.Attr) {
	slog.LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
}

func Error(msg string, attrs ...slog.Attr) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // Skip down the stack so we don't say the error is from this function
	r := slog.NewRecord(time.Now(), slog.LevelError, msg, pcs[0])
	r.AddAttrs(attrs...)

	_ = errorLogger.Handler().Handle(context.Background(), r)
}

func GetDefaultLogger() *slog.Logger {
	return defaultLogger
}
