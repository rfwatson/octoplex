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
	"github.com/gdamore/tcell/v2"
)

// RunParams holds the parameters for running the application.
type RunParams struct {
	Config             config.Config
	DockerClient       container.DockerClient
	Screen             tcell.Screen
	ClipboardAvailable bool
	BuildInfo          domain.BuildInfo
	Logger             *slog.Logger
}

// Run starts the application, and blocks until it exits.
func Run(ctx context.Context, params RunParams) error {
	state := newStateFromRunParams(params)
	logger := params.Logger

	ui, err := terminal.StartUI(ctx, terminal.StartParams{
		Screen:             params.Screen,
		ClipboardAvailable: params.ClipboardAvailable,
		BuildInfo:          params.BuildInfo,
		Logger:             logger.With("component", "ui"),
	})
	if err != nil {
		return fmt.Errorf("start terminal user interface: %w", err)
	}
	defer ui.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

	containerClient, err := container.NewClient(ctx, params.DockerClient, logger.With("component", "container_client"))
	if err != nil {
		return fmt.Errorf("new container client: %w", err)
	}
	defer containerClient.Close()

	if exists, err := containerClient.ContainerRunning(ctx, container.AllContainers()); err != nil {
		return fmt.Errorf("check existing containers: %w", err)
	} else if exists {
		if ui.ShowStartupCheckModal() {
			if err = containerClient.RemoveContainers(ctx, container.AllContainers()); err != nil {
				return fmt.Errorf("remove existing containers: %w", err)
			}
		} else {
			return nil
		}
	}
	ui.AllowQuit()

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

	const uiUpdateInterval = time.Second
	uiUpdateT := time.NewTicker(uiUpdateInterval)
	defer uiUpdateT.Stop()

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
			case terminal.CommandStartDestination:
				mp.StartDestination(c.URL)
			case terminal.CommandStopDestination:
				mp.StopDestination(c.URL)
			case terminal.CommandQuit:
				return nil
			}
		case <-uiUpdateT.C:
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

// newStateFromRunParams creates a new app state from the run parameters.
func newStateFromRunParams(params RunParams) *domain.AppState {
	var state domain.AppState

	state.Destinations = make([]domain.Destination, 0, len(params.Config.Destinations))
	for _, dest := range params.Config.Destinations {
		state.Destinations = append(state.Destinations, domain.Destination{
			Name: dest.Name,
			URL:  dest.URL,
		})
	}

	return &state
}
