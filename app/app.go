package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"git.netflux.io/rob/octoplex/config"
	"git.netflux.io/rob/octoplex/container"
	"git.netflux.io/rob/octoplex/domain"
	"git.netflux.io/rob/octoplex/mediaserver"
	"git.netflux.io/rob/octoplex/multiplexer"
	"git.netflux.io/rob/octoplex/terminal"
)

const uiUpdateInterval = 2 * time.Second

// Run starts the application, and blocks until it exits.
func Run(
	ctx context.Context,
	cfg config.Config,
	dockerClient container.DockerClient,
	clipboardAvailable bool,
	logger *slog.Logger,
) error {
	state := new(domain.AppState)
	applyConfig(cfg, state)

	ui, err := terminal.StartActor(ctx, terminal.StartActorParams{
		ClipboardAvailable: clipboardAvailable,
		Logger:             logger.With("component", "ui"),
	})
	if err != nil {
		return fmt.Errorf("start tui: %w", err)
	}
	defer ui.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

	containerClient, err := container.NewClient(ctx, dockerClient, logger.With("component", "container_client"))
	if err != nil {
		return fmt.Errorf("new container client: %w", err)
	}
	defer containerClient.Close()

	srv := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		ContainerClient: containerClient,
		Logger:          logger.With("component", "mediaserver"),
	})
	defer srv.Close()

	mp := multiplexer.NewActor(ctx, multiplexer.NewActorParams{
		SourceURL:       srv.State().RTMPInternalURL,
		ContainerClient: containerClient,
		Logger:          logger.With("component", "multiplexer"),
	})
	defer mp.Close()

	uiTicker := time.NewTicker(uiUpdateInterval)
	defer uiTicker.Stop()

	for {
		select {
		case cmd, ok := <-ui.C():
			if !ok {
				// TODO: keep UI open until all containers have closed
				logger.Info("UI closed")
				return nil
			}

			logger.Info("Command received", "cmd", cmd.Name())
			switch c := cmd.(type) {
			case terminal.CommandToggleDestination:
				mp.ToggleDestination(c.URL)
			case terminal.CommandQuit:
				return nil
			}
		case <-uiTicker.C:
			updateUI()
		case serverState := <-srv.C():
			applyServerState(serverState, state)
			updateUI()
		case mpState := <-mp.C():
			applyMultiplexerState(mpState, state)
			updateUI()
		}
	}
}

// applyServerState applies the current server state to the app state.
func applyServerState(serverState domain.Source, appState *domain.AppState) {
	appState.Source = serverState
}

// applyMultiplexerState applies the current multiplexer state to the app state.
func applyMultiplexerState(mpState multiplexer.State, appState *domain.AppState) {
	for i, dest := range appState.Destinations {
		if dest.URL != mpState.URL {
			continue
		}

		appState.Destinations[i].Container = mpState.Container
		appState.Destinations[i].Status = mpState.Status

		break
	}
}

// applyConfig applies the configuration to the app state.
func applyConfig(cfg config.Config, appState *domain.AppState) {
	appState.Destinations = make([]domain.Destination, 0, len(cfg.Destinations))
	for _, dest := range cfg.Destinations {
		appState.Destinations = append(appState.Destinations, domain.Destination{
			Name: dest.Name,
			URL:  dest.URL,
		})
	}
}
