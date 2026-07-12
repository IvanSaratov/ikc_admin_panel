package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config описывает runtime-настройки logger. Пустые значения безопасны:
// локальная разработка получает text/info, production получает JSON/info.
type Config struct {
	Env    string
	Level  string
	Format string
	Output io.Writer
}

// New создаёт изолированный zap.Logger для приложения.
func New(cfg Config) (*zap.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	format, err := resolveFormat(cfg.Env, cfg.Format)
	if err != nil {
		return nil, err
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeDuration = zapcore.MillisDurationEncoder

	var encoder zapcore.Encoder
	switch format {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	case "text":
		encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(out), level)
	return zap.New(core), nil
}

func parseLevel(raw string) (zapcore.Level, error) {
	if strings.TrimSpace(raw) == "" {
		return zapcore.InfoLevel, nil
	}
	var level zapcore.Level
	if err := level.Set(strings.ToLower(strings.TrimSpace(raw))); err != nil {
		return zapcore.InfoLevel, fmt.Errorf("parse log level %q: %w", raw, err)
	}
	return level, nil
}

func resolveFormat(env string, format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		switch strings.ToLower(strings.TrimSpace(env)) {
		case "prod", "production":
			format = "json"
		default:
			format = "text"
		}
	}

	switch format {
	case "json", "text":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported log format %q", format)
	}
}
