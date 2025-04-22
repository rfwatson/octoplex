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
	"git.netflux.io/rob/octoplex/internal/event"
	"git.netflux.io/rob/octoplex/internal/mediaserver"
	"git.netflux.io/rob/octoplex/internal/replicator"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"github.com/docker/docker/client"
)

// App is an instance of the app.
type App struct {
	configService      *config.Service
	dockerClient       container.DockerClient
	screen             *terminal.Screen // Screen may be nil.
	clipboardAvailable bool
	configFilePath     string
	buildInfo          domain.BuildInfo
	logger             *slog.Logger
}

// Params holds the parameters for running the application.
type Params struct {
	ConfigService      *config.Service
	DockerClient       container.DockerClient
	Screen             *terminal.Screen // Screen may be nil.
	ClipboardAvailable bool
	ConfigFilePath     string
	BuildInfo          domain.BuildInfo
	Logger             *slog.Logger
}

func New(params Params) *App {
	return &App{
		configService:      params.ConfigService,
		dockerClient:       params.DockerClient,
		screen:             params.Screen,
		clipboardAvailable: params.ClipboardAvailable,
		configFilePath:     params.ConfigFilePath,
		buildInfo:          params.BuildInfo,
		logger:             params.Logger,
	}
}

// Run starts the application, and blocks until it exits.
func (a *App) Run(ctx context.Context) error {
	eventBus := event.NewBus(a.logger.With("component", "event_bus"))

	// cfg is the current configuration of the application, as reflected in the
	// config file.
	cfg := a.configService.Current()

	// state is the current state of the application, as reflected in the UI.
	state := new(domain.AppState)
	applyConfig(cfg, state)

	// Ensure there is at least one active source.
	if !cfg.Sources.MediaServer.RTMP.Enabled && !cfg.Sources.MediaServer.RTMPS.Enabled {
		return errors.New("config: either sources.mediaServer.rtmp.enabled or sources.mediaServer.rtmps.enabled must be set")
	}

	ui, err := terminal.StartUI(ctx, terminal.StartParams{
		EventBus:           eventBus,
		Screen:             a.screen,
		ClipboardAvailable: a.clipboardAvailable,
		ConfigFilePath:     a.configFilePath,
		BuildInfo:          a.buildInfo,
		Logger:             a.logger.With("component", "ui"),
	})
	if err != nil {
		return fmt.Errorf("start terminal user interface: %w", err)
	}
	defer ui.Close()

	// emptyUI is a dummy function that sets the UI state to an empty state, and
	// re-renders the screen.
	//
	// This is a workaround for a weird interaction between tview and
	// tcell.SimulationScreen which leads to newly-added pages not rendering if
	// the UI is not re-rendered for a second time.
	// It is only needed for integration tests when rendering modals before the
	// main loop starts. It would be nice to remove this but the risk/impact on
	// non-test code is pretty low.
	emptyUI := func() { ui.SetState(domain.AppState{}) }

	containerClient, err := container.NewClient(ctx, a.dockerClient, a.logger.With("component", "container_client"))
	if err != nil {
		err = fmt.Errorf("create container client: %w", err)

		var errString string
		if client.IsErrConnectionFailed(err) {
			errString = "Could not connect to Docker. Is Docker installed and running?"
		} else {
			errString = err.Error()
		}
		ui.ShowFatalErrorModal(errString)

		emptyUI()
		<-ui.C()
		return err
	}
	defer containerClient.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

	var tlsCertPath, tlsKeyPath string
	if cfg.Sources.MediaServer.TLS != nil {
		tlsCertPath = cfg.Sources.MediaServer.TLS.CertPath
		tlsKeyPath = cfg.Sources.MediaServer.TLS.KeyPath
	}

	srv, err := mediaserver.NewActor(ctx, mediaserver.NewActorParams{
		RTMPAddr:        buildNetAddr(cfg.Sources.MediaServer.RTMP),
		RTMPSAddr:       buildNetAddr(cfg.Sources.MediaServer.RTMPS),
		Host:            cfg.Sources.MediaServer.Host,
		TLSCertPath:     tlsCertPath,
		TLSKeyPath:      tlsKeyPath,
		StreamKey:       mediaserver.StreamKey(cfg.Sources.MediaServer.StreamKey),
		ContainerClient: containerClient,
		Logger:          a.logger.With("component", "mediaserver"),
	})
	if err != nil {
		err = fmt.Errorf("create mediaserver: %w", err)
		ui.ShowFatalErrorModal(err.Error())
		emptyUI()
		<-ui.C()
		return err
	}
	defer srv.Close()

	repl := replicator.StartActor(ctx, replicator.StartActorParams{
		SourceURL:       srv.RTMPInternalURL(),
		ContainerClient: containerClient,
		Logger:          a.logger.With("component", "replicator"),
	})
	defer repl.Close()

	const uiUpdateInterval = time.Second
	uiUpdateT := time.NewTicker(uiUpdateInterval)
	defer uiUpdateT.Stop()

	startupCheckC := doStartupCheck(ctx, containerClient, ui.ShowStartupCheckModal)

	for {
		select {
		case err := <-startupCheckC:
			if errors.Is(err, errStartupCheckUserQuit) {
				return nil
			} else if err != nil {
				return fmt.Errorf("startup check: %w", err)
			} else {
				startupCheckC = nil

				if err = srv.Start(ctx); err != nil {
					return fmt.Errorf("start mediaserver: %w", err)
				}

				eventBus.Send(event.MediaServerStartedEvent{RTMPURL: srv.RTMPURL(), RTMPSURL: srv.RTMPSURL()})
			}
		case <-a.configService.C():
			// No-op, config updates are handled synchronously for now.
		case cmd, ok := <-ui.C():
			if !ok {
				// TODO: keep UI open until all containers have closed
				a.logger.Info("UI closed")
				return nil
			}

			a.logger.Debug("Command received", "cmd", cmd.Name())
			switch c := cmd.(type) {
			case domain.CommandAddDestination:
				newCfg := cfg
				newCfg.Destinations = append(newCfg.Destinations, config.Destination{
					Name: c.DestinationName,
					URL:  c.URL,
				})
				if err := a.configService.SetConfig(newCfg); err != nil {
					a.logger.Error("Config update failed", "err", err)
					ui.ConfigUpdateFailed(err)
					continue
				}
				cfg = newCfg
				handleConfigUpdate(cfg, state, ui)
				ui.DestinationAdded()
			case domain.CommandRemoveDestination:
				repl.StopDestination(c.URL) // no-op if not live
				newCfg := cfg
				newCfg.Destinations = slices.DeleteFunc(newCfg.Destinations, func(dest config.Destination) bool {
					return dest.URL == c.URL
				})
				if err := a.configService.SetConfig(newCfg); err != nil {
					a.logger.Error("Config update failed", "err", err)
					ui.ConfigUpdateFailed(err)
					continue
				}
				cfg = newCfg
				handleConfigUpdate(cfg, state, ui)
				ui.DestinationRemoved()
			case domain.CommandStartDestination:
				if !state.Source.Live {
					ui.ShowSourceNotLiveModal()
					continue
				}

				repl.StartDestination(c.URL)
			case domain.CommandStopDestination:
				repl.StopDestination(c.URL)
			case domain.CommandQuit:
				return nil
			}
		case <-uiUpdateT.C:
			updateUI()
		case serverState := <-srv.C():
			a.logger.Debug("Server state received", "state", serverState)
			applyServerState(serverState, state)
			updateUI()
		case replState := <-repl.C():
			a.logger.Debug("Replicator state received", "state", replState)
			destErrors := applyReplicatorState(replState, state)

			for _, destError := range destErrors {
				handleDestError(destError, repl, ui)
			}

			updateUI()
		}
	}
}

// handleConfigUpdate applies the config to the app state, and updates the UI.
func handleConfigUpdate(cfg config.Config, appState *domain.AppState, ui *terminal.UI) {
	applyConfig(cfg, appState)
	ui.SetState(*appState)
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

var errStartupCheckUserQuit = errors.New("user quit startup check modal")

// doStartupCheck performs a startup check to see if there are any existing app
// containers.
//
// It returns a channel that will be closed, possibly after receiving an error.
// If the error is non-nil the app must not be started. If the error is
// [errStartupCheckUserQuit], the user voluntarily quit the startup check
// modal.
func doStartupCheck(ctx context.Context, containerClient *container.Client, showModal func() bool) <-chan error {
	ch := make(chan error, 1)

	go func() {
		defer close(ch)

		if exists, err := containerClient.ContainerRunning(ctx, container.AllContainers()); err != nil {
			ch <- fmt.Errorf("check existing containers: %w", err)
		} else if exists {
			if showModal() {
				if err = containerClient.RemoveContainers(ctx, container.AllContainers()); err != nil {
					ch <- fmt.Errorf("remove existing containers: %w", err)
					return
				}
				if err = containerClient.RemoveUnusedNetworks(ctx); err != nil {
					ch <- fmt.Errorf("remove unused networks: %w", err)
					return
				}
			} else {
				ch <- errStartupCheckUserQuit
			}
		}
	}()

	return ch
}

// buildNetAddr builds a [mediaserver.OptionalNetAddr] from the config.
func buildNetAddr(src config.RTMPSource) mediaserver.OptionalNetAddr {
	if !src.Enabled {
		return mediaserver.OptionalNetAddr{Enabled: false}
	}

	return mediaserver.OptionalNetAddr{Enabled: true, NetAddr: domain.NetAddr(src.NetAddr)}
}
