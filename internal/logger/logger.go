package logger

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(logLevel string) *zap.Logger {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(logLevel)); err != nil {
		lvl = zapcore.InfoLevel
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Encoding:         "json",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig:    zap.NewProductionEncoderConfig(),
	}

	logger, err := cfg.Build()
	if err != nil {
		log.Fatalf("не удалось создать логгер: %v\n", err)
	}

	return logger
}
