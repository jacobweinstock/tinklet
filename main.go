package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jacobweinstock/tinklet/cmd"
)

func main() {
	exitCode := 0
	defer func() {
		os.Exit(exitCode)
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())

	defer func() {
		signal.Stop(signals)
		cancel()
	}()

	go func() {
		select {
		case <-signals:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := cmd.Execute(ctx); err != nil {
		fmt.Printf(`{"level":"error","msg":"tinklet failed","err":"%v"}`, err)
		fmt.Println()
		exitCode = 1
	}
}
