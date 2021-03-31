package cmd

import (
	"context"

	"github.com/jacobweinstock/goconfig"
	"github.com/jacobweinstock/tinklet/internal"
	"github.com/packethost/pkg/log/logr"
)

func Execute(ctx context.Context) error {
	// setup config and logging, then run the tinklet control loop
	config := internal.Configuration{
		LogLevel: "info",
	}
	cfgParser := goconfig.NewParser(
		goconfig.WithPrefix("TINKLET"),
		goconfig.WithFile("tinklet.example.yaml"),
	)
	cfgParser.Parse(&config)

	log, _, _ := logr.NewPacketLogr(
		logr.WithServiceName("tinklet"),
		logr.WithLogLevel(config.LogLevel),
	)
	log.V(0).Info("starting tinklet control loop")
	return internal.RunControlLoop(ctx, log, config)
}
