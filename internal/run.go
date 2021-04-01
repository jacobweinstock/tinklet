package internal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
)

// RunControlLoop runs the tinklet control loop that watchs for workflows and runs them
func RunControlLoop(ctx context.Context, log logr.Logger, config Configuration) error {
	log.V(0).Info("debugging", "config", config)
	for {
		select {
		case <-ctx.Done():
			log.V(0).Info("stopping control loop")
			return nil
		default:
		}
		time.Sleep(1 * time.Second)
	}
}
