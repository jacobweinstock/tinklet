package cmd

import (
	"context"

	"github.com/jacobweinstock/goconfig"
	"github.com/jacobweinstock/tinklet/internal"
	"github.com/packethost/pkg/log/logr"
	"github.com/philippgille/gokv/freecache"
)

// Execute sets up the config and logging, then run the tinklet control loop
func Execute(ctx context.Context) error {
	// set default config values
	config := internal.Configuration{
		LogLevel: "info",
	}
	cfgParser := goconfig.NewParser(
		goconfig.WithPrefix("TINKLET"),
		goconfig.WithFile("tinklet.yaml"),
	)
	err := cfgParser.Parse(&config)
	if err != nil {
		return err
	}

	log, _, _ := logr.NewPacketLogr(
		logr.WithServiceName("tinklet"),
		logr.WithLogLevel(config.LogLevel),
	)
	log.V(0).Info("starting tinklet control loop")

	return internal.RunControlLoop(ctx, log, config, freecache.NewStore(freecache.DefaultOptions))
}
