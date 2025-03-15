package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/mediaserver"
	"git.netflux.io/rob/octoplex/internal/multiplexer"
	"git.netflux.io/rob/octoplex/internal/terminal"
)

// RunParams holds the parameters for running the application.
type RunParams struct {
	Config             config.Config
	DockerClient       container.DockerClient
	Screen             *terminal.Screen // Screen may be nil.
	ClipboardAvailable bool
	ConfigFilePath     string
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
		ConfigFilePath:     params.ConfigFilePath,
		BuildInfo:          params.BuildInfo,
		Logger:             logger.With("component", "ui"),
	})
	if err != nil {
		return fmt.Errorf("start terminal user interface: %w", err)
	}
	defer ui.Close()

	containerClient, err := container.NewClient(ctx, params.DockerClient, logger.With("component", "container_client"))
	if err != nil {
		return err
	}
	defer containerClient.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

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
			destErrors := applyMultiplexerState(mpState, state)

			for _, destError := range destErrors {
				handleDestError(destError, mp, ui)
			}

			updateUI()
		}
	}
}

// applyServerState applies the current server state to the app state.
func applyServerState(serverState domain.Source, appState *domain.AppState) {
	appState.Source = serverState
}

// destinationError holds the information needed to display a destination
// error.
type destinationError struct {
	name string
	url  string
	err  error
}

// applyMultiplexerState applies the current multiplexer state to the app state.
//
// It returns a list of destination errors that should be displayed to the user.
func applyMultiplexerState(mpState multiplexer.State, appState *domain.AppState) []destinationError {
	var errorsToDisplay []destinationError

	for i := range appState.Destinations {
		dest := &appState.Destinations[i]

		if dest.URL != mpState.URL {
			continue
		}

		if dest.Container.Err == nil && mpState.Container.Err != nil {
			errorsToDisplay = append(errorsToDisplay, destinationError{
				name: dest.Name,
				url:  dest.URL,
				err:  mpState.Container.Err,
			})
		}

		dest.Container = mpState.Container
		dest.Status = mpState.Status

		break
	}

	return errorsToDisplay
}

// handleDestError displays a modal to the user, and stops the destination.
func handleDestError(destError destinationError, mp *multiplexer.Actor, ui *terminal.UI) {
	ui.ShowDestinationErrorModal(destError.name, destError.err)

	mp.StopDestination(destError.url)
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
