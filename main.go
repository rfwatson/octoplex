package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"git.netflux.io/rob/termstream/config"
	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/domain"
	"git.netflux.io/rob/termstream/mediaserver"
	"git.netflux.io/rob/termstream/terminal"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, config.FromFile()); err != nil {
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

func run(ctx context.Context, cfgReader io.Reader) error {
	cfg, err := config.Load(cfgReader)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	state := new(domain.AppState)
	applyConfig(cfg, state)

	logFile, err := os.OpenFile("termstream.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(logFile, nil))
	logger.Info("Starting termstream", slog.Any("initial_state", state))

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
	applyServerState(srv.State(), state)

	ui, err := terminal.StartActor(ctx, terminal.StartActorParams{Logger: logger.With("component", "ui")})
	if err != nil {
		return fmt.Errorf("start tui: %w", err)
	}
	defer ui.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

	for {
		select {
		case cmd, ok := <-ui.C():
			logger.Info("Command received", "cmd", cmd)
			if !ok {
				logger.Info("UI closed")
				return nil
			}
		case serverState, ok := <-srv.C():
			if ok {
				applyServerState(serverState, state)
				updateUI()
			} else {
				logger.Info("State channel closed, shutting down...")
				return nil
			}
		}
	}
}

// applyServerState applies the current server state to the app state.
func applyServerState(serverState mediaserver.State, appState *domain.AppState) {
	appState.ContainerRunning = serverState.ContainerRunning
	appState.IngressLive = serverState.IngressLive
	appState.IngressURL = serverState.IngressURL
}

// applyConfig applies the configuration to the app state.
func applyConfig(cfg config.Config, appState *domain.AppState) {
	appState.Destinations = make([]domain.Destination, 0, len(cfg.Destinations))
	for _, dest := range cfg.Destinations {
		appState.Destinations = append(appState.Destinations, domain.Destination{URL: dest.URL})
	}
}
