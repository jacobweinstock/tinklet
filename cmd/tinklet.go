package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/go-playground/validator/v10"
	"github.com/jacobweinstock/tinklet/cmd/docker"
	"github.com/jacobweinstock/tinklet/cmd/kube"
	"github.com/jacobweinstock/tinklet/cmd/root"
	"github.com/peterbourgon/ff/v3/ffcli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Execute sets up the config and logging, then run the tinklet control loop
func Execute(ctx context.Context) error {
	var (
		rootCommand, rootConfig = root.New()
		dockerCommand           = docker.New(rootConfig)
		kubeCommand             = kube.New(rootConfig)
	)

	rootCommand.Subcommands = []*ffcli.Command{
		dockerCommand,
		kubeCommand,
	}

	if err := rootCommand.Parse(os.Args[1:]); err != nil {
		return err
	}

	if err := validator.New().Struct(rootConfig); err != nil {
		return err
	}
	rootConfig.Log = defaultLogger(rootConfig.LogLevel)

	if err := rootCommand.Run(ctx); err != nil {
		return err
	}
	return nil
}

// defaultLogger is zap logr implementation
func defaultLogger(level string) logr.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
	zapLogger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}

	return zapr.NewLogger(zapLogger)
}
