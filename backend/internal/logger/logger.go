package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.Logger

// Init initialises the global zap logger. Call once at startup.
func Init(env string) {
	var cfg zapcore.EncoderConfig
	var level zapcore.Level

	if env == "production" {
		cfg = zap.NewProductionEncoderConfig()
		level = zapcore.InfoLevel
	} else {
		cfg = zap.NewDevelopmentEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		level = zapcore.DebugLevel
	}

	cfg.TimeKey = "timestamp"
	cfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format(time.RFC3339Nano))
	}

	var encoder zapcore.Encoder
	if env == "production" {
		encoder = zapcore.NewJSONEncoder(cfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(cfg)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		level,
	)
	log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

// Get returns the global logger. Init must be called first.
func Get() *zap.Logger {
	if log == nil {
		// Fallback: safe no-op
		log, _ = zap.NewDevelopment()
	}
	return log
}

// Sync flushes buffered log entries – defer in main().
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}
