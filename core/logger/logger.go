package logger

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ctxKey string

const loggerKey ctxKey = "logger"

func Init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		NoColor:    false,
	})
}

func ToContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) zerolog.Logger {
	logger, ok := ctx.Value(loggerKey).(zerolog.Logger)
	if !ok {
		return log.Logger
	}
	return logger
}
