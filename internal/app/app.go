package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/mediaserver"
	"git.netflux.io/rob/octoplex/internal/replicator"
	"git.netflux.io/rob/octoplex/internal/terminal"
)

// RunParams holds the parameters for running the application.
type RunParams struct {
	ConfigService      *config.Service
	DockerClient       container.DockerClient
	Screen             *terminal.Screen // Screen may be nil.
	ClipboardAvailable bool
	ConfigFilePath     string
	BuildInfo          domain.BuildInfo
	Logger             *slog.Logger
}

// Run starts the application, and blocks until it exits.
func Run(ctx context.Context, params RunParams) error {
	// cfg is the current configuration of the application, as reflected in the
	// config file.
	cfg := params.ConfigService.Current()

	// state is the current state of the application, as reflected in the UI.
	state := new(domain.AppState)
	applyConfig(cfg, state)

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

	var exists bool
	if exists, err = containerClient.ContainerRunning(ctx, container.AllContainers()); err != nil {
		return fmt.Errorf("check existing containers: %w", err)
	} else if exists {
		if ui.ShowStartupCheckModal() {
			if err = containerClient.RemoveContainers(ctx, container.AllContainers()); err != nil {
				return fmt.Errorf("remove existing containers: %w", err)
			}
			if err = containerClient.RemoveUnusedNetworks(ctx); err != nil {
				return fmt.Errorf("remove unused networks: %w", err)
			}
		} else {
			return nil
		}
	}
	ui.AllowQuit()

	// While RTMP is the only source, it doesn't make sense to disable it.
	if !cfg.Sources.RTMP.Enabled {
		return errors.New("config: sources.rtmp.enabled must be set to true")
	}

	srv, err := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		StreamKey:       mediaserver.StreamKey(cfg.Sources.RTMP.StreamKey),
		ContainerClient: containerClient,
		Logger:          logger.With("component", "mediaserver"),
	})
	if err != nil {
		return fmt.Errorf("start mediaserver: %w", err)
	}
	defer srv.Close()

	repl := replicator.NewActor(ctx, replicator.NewActorParams{
		SourceURL:       srv.State().RTMPInternalURL,
		ContainerClient: containerClient,
		Logger:          logger.With("component", "replicator"),
	})
	defer repl.Close()

	const uiUpdateInterval = time.Second
	uiUpdateT := time.NewTicker(uiUpdateInterval)
	defer uiUpdateT.Stop()

	for {
		select {
		case cfg = <-params.ConfigService.C():
			applyConfig(cfg, state)
			updateUI()
		case cmd, ok := <-ui.C():
			if !ok {
				// TODO: keep UI open until all containers have closed
				logger.Info("UI closed")
				return nil
			}

			logger.Debug("Command received", "cmd", cmd.Name())
			switch c := cmd.(type) {
			case terminal.CommandAddDestination:
				newCfg := cfg
				newCfg.Destinations = append(newCfg.Destinations, config.Destination{
					Name: c.DestinationName,
					URL:  c.URL,
				})
				if err := params.ConfigService.SetConfig(newCfg); err != nil {
					logger.Error("Config update failed", "err", err)
					ui.ConfigUpdateFailed(err)
					continue
				}
				ui.DestinationAdded()
			case terminal.CommandRemoveDestination:
				repl.StopDestination(c.URL) // no-op if not live
				newCfg := cfg
				newCfg.Destinations = slices.DeleteFunc(newCfg.Destinations, func(dest config.Destination) bool {
					return dest.URL == c.URL
				})
				if err := params.ConfigService.SetConfig(newCfg); err != nil {
					logger.Error("Config update failed", "err", err)
					ui.ConfigUpdateFailed(err)
					continue
				}
			case terminal.CommandStartDestination:
				if !state.Source.Live {
					ui.ShowSourceNotLiveModal()
					continue
				}

				repl.StartDestination(c.URL)
			case terminal.CommandStopDestination:
				repl.StopDestination(c.URL)
			case terminal.CommandQuit:
				return nil
			}
		case <-uiUpdateT.C:
			updateUI()
		case serverState := <-srv.C():
			logger.Debug("Server state received", "state", serverState)
			applyServerState(serverState, state)
			updateUI()
		case replState := <-repl.C():
			logger.Debug("Replicator state received", "state", replState)
			destErrors := applyReplicatorState(replState, state)

			for _, destError := range destErrors {
				handleDestError(destError, repl, ui)
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

// applyReplicatorState applies the current replicator state to the app state.
//
// It returns a list of destination errors that should be displayed to the user.
func applyReplicatorState(replState replicator.State, appState *domain.AppState) []destinationError {
	var errorsToDisplay []destinationError

	for i := range appState.Destinations {
		dest := &appState.Destinations[i]

		if dest.URL != replState.URL {
			continue
		}

		if dest.Container.Err == nil && replState.Container.Err != nil {
			errorsToDisplay = append(errorsToDisplay, destinationError{
				name: dest.Name,
				url:  dest.URL,
				err:  replState.Container.Err,
			})
		}

		dest.Container = replState.Container
		dest.Status = replState.Status

		break
	}

	return errorsToDisplay
}

// handleDestError displays a modal to the user, and stops the destination.
func handleDestError(destError destinationError, repl *replicator.Actor, ui *terminal.UI) {
	ui.ShowDestinationErrorModal(destError.name, destError.err)

	repl.StopDestination(destError.url)
}

// applyConfig applies the config to the app state. For now we only set the
// destinations.
func applyConfig(cfg config.Config, appState *domain.AppState) {
	appState.Destinations = resolveDestinations(appState.Destinations, cfg.Destinations)
}

// resolveDestinations merges the current destinations with newly configured
// destinations.
func resolveDestinations(destinations []domain.Destination, inDestinations []config.Destination) []domain.Destination {
	destinations = slices.DeleteFunc(destinations, func(dest domain.Destination) bool {
		return !slices.ContainsFunc(inDestinations, func(inDest config.Destination) bool {
			return inDest.URL == dest.URL
		})
	})

	for i, inDest := range inDestinations {
		if i < len(destinations) && destinations[i].URL == inDest.URL {
			continue
		}

		destinations = slices.Insert(destinations, i, domain.Destination{
			Name: inDest.Name,
			URL:  inDest.URL,
		})
	}

	return destinations[:len(inDestinations)]
}
