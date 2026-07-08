package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Config описывает runtime-настройки logger. Пустые значения безопасны:
// локальная разработка получает text/info, production получает JSON/info.
type Config struct {
	Env    string
	Level  string
	Format string
	Output io.Writer
}

// New создаёт изолированный logrus.Logger для приложения.
func New(cfg Config) (*logrus.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	formatter, err := formatter(cfg.Env, cfg.Format)
	if err != nil {
		return nil, err
	}
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	logger := logrus.New()
	logger.SetOutput(out)
	logger.SetLevel(level)
	logger.SetFormatter(formatter)
	return logger, nil
}

func parseLevel(raw string) (logrus.Level, error) {
	if strings.TrimSpace(raw) == "" {
		return logrus.InfoLevel, nil
	}
	level, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return logrus.InfoLevel, fmt.Errorf("parse log level %q: %w", raw, err)
	}
	return level, nil
}

func formatter(env string, format string) (logrus.Formatter, error) {
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
	case "json":
		return &logrus.JSONFormatter{}, nil
	case "text":
		return &logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}
