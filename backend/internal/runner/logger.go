package runner

import (
	"io"

	"github.com/IvanSaratov/ikc_admin_panel/backend/platform/logging"
	"go.uber.org/zap"
)

func withLogger(config RuntimeConfig, output io.Writer, action func(*zap.Logger) error) error {
	logger, err := logging.New(logging.Config{
		Env:    config.Environment,
		Level:  config.LogLevel,
		Format: config.LogFormat,
		Output: output,
	})
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()
	undoGlobals := zap.ReplaceGlobals(logger)
	defer undoGlobals()
	restoreStdLog := zap.RedirectStdLog(logger.Named("stdlog"))
	defer restoreStdLog()
	return action(logger)
}
