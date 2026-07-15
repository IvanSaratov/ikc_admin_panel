package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/IvanSaratov/ikc_admin_panel/backend/platform/logging"
	"go.uber.org/zap"
)

func main() {
	logger, err := newLoggerFromEnv(os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL: invalid log configuration")
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	undoGlobals := zap.ReplaceGlobals(logger)
	defer undoGlobals()

	restoreStdLog := zap.RedirectStdLog(logger.Named("stdlog"))
	defer restoreStdLog()

	if err := runCommand(context.Background(), os.Args[1:], os.Stdout, logger); err != nil {
		logger.Error("Mintrud Admin stopped with error", zap.Error(err))
		os.Exit(1)
	}
}

func newLoggerFromEnv(output io.Writer) (*zap.Logger, error) {
	return logging.New(logging.Config{
		Env:    os.Getenv("MINTRUD_ADMIN_ENV"),
		Level:  os.Getenv("MINTRUD_ADMIN_LOG_LEVEL"),
		Format: os.Getenv("MINTRUD_ADMIN_LOG_FORMAT"),
		Output: output,
	})
}
