package main

import (
	"context"
	"errors"
	"io"

	"go.uber.org/zap"
)

var ErrUsage = errors.New("usage: mintrud-admin [serve|db <status|migrate|verify|backup>]")

func runCommand(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	logger *zap.Logger,
) error {
	serveCommand := len(args) == 0 || (len(args) == 1 && args[0] == "serve")
	databaseCommand := len(args) == 2 && args[0] == "db" && isDatabaseAction(args[1])
	if !serveCommand && !databaseCommand {
		return ErrUsage
	}

	config, err := loadRuntimeConfig()
	if err != nil {
		return err
	}
	if serveCommand {
		return runServe(ctx, config, logger)
	}
	return runDatabaseCommand(ctx, args[1], config, stdout, logger)
}

func isDatabaseAction(action string) bool {
	switch action {
	case "status", "migrate", "verify", "backup":
		return true
	default:
		return false
	}
}
