package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/mediaserver"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

func run(ctx context.Context) error {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	runner, err := container.NewRunner(logger.With("component", "runner"))
	if err != nil {
		return fmt.Errorf("new runner: %w", err)
	}
	defer runner.Close()

	srv, err := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		Runner: runner,
		Logger: logger.With("component", "mediaserver"),
	})
	if err != nil {
		return fmt.Errorf("start media server: %w", err)
	}

	for {
		select {
		case <-ch:
			logger.Info("Received interrupt signal, shutting down...")
			return nil
		case state, ok := <-srv.C():
			if ok {
				logger.Info("Received state change", "state", state)
			} else {
				logger.Info("State channel closed, shutting down...")
				return nil
			}
		}
	}
}
