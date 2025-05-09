package app

import (
	"cmp"
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
	cfg                config.Config
	configService      *config.Service
	eventBus           *event.Bus
	dispatchC          chan event.Command
	dockerClient       container.DockerClient
	screen             *terminal.Screen // Screen may be nil.
	headless           bool
	clipboardAvailable bool
	configFilePath     string
	buildInfo          domain.BuildInfo
	logger             *slog.Logger
}

// Params holds the parameters for running the application.
type Params struct {
	ConfigService      *config.Service
	DockerClient       container.DockerClient
	ChanSize           int
	Screen             *terminal.Screen // Screen may be nil.
	Headless           bool
	ClipboardAvailable bool
	ConfigFilePath     string
	BuildInfo          domain.BuildInfo
	Logger             *slog.Logger
}

// defaultChanSize is the default size of the dispatch channel.
const defaultChanSize = 64

// New creates a new application instance.
func New(params Params) *App {
	return &App{
		cfg:                params.ConfigService.Current(),
		configService:      params.ConfigService,
		eventBus:           event.NewBus(params.Logger.With("component", "event_bus")),
		dispatchC:          make(chan event.Command, cmp.Or(params.ChanSize, defaultChanSize)),
		dockerClient:       params.DockerClient,
		screen:             params.Screen,
		headless:           params.Headless,
		clipboardAvailable: params.ClipboardAvailable,
		configFilePath:     params.ConfigFilePath,
		buildInfo:          params.BuildInfo,
		logger:             params.Logger,
	}
}

// Run starts the application, and blocks until it exits.
func (a *App) Run(ctx context.Context) error {
	// state is the current state of the application, as reflected in the UI.
	state := new(domain.AppState)
	applyConfig(a.cfg, state)

	// Ensure there is at least one active source.
	if !a.cfg.Sources.MediaServer.RTMP.Enabled && !a.cfg.Sources.MediaServer.RTMPS.Enabled {
		return errors.New("config: either sources.mediaServer.rtmp.enabled or sources.mediaServer.rtmps.enabled must be set")
	}

	if !a.headless {
		ui, err := terminal.StartUI(ctx, terminal.StartParams{
			EventBus:           a.eventBus,
			Dispatcher:         func(cmd event.Command) { a.dispatchC <- cmd },
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
	}

	// emptyUI is a dummy function that sets the UI state to an empty state, and
	// re-renders the screen.
	//
	// This is a workaround for a weird interaction between tview and
	// tcell.SimulationScreen which leads to newly-added pages not rendering if
	// the UI is not re-rendered for a second time.
	// It is only needed for integration tests when rendering modals before the
	// main loop starts. It would be nice to remove this but the risk/impact on
	// non-test code is pretty low.
	emptyUI := func() {
		a.eventBus.Send(event.AppStateChangedEvent{State: domain.AppState{}})
	}

	// doFatalError publishes a fatal error to the event bus, waiting for the
	// user to acknowledge it if not in headless mode.
	doFatalError := func(msg string) {
		a.eventBus.Send(event.FatalErrorOccurredEvent{Message: msg})

		if a.headless {
			return
		}

		emptyUI()
		<-a.dispatchC
	}

	containerClient, err := container.NewClient(ctx, a.dockerClient, a.logger.With("component", "container_client"))
	if err != nil {
		err = fmt.Errorf("create container client: %w", err)

		var msg string
		if client.IsErrConnectionFailed(err) {
			msg = "Could not connect to Docker. Is Docker installed and running?"
		} else {
			msg = err.Error()
		}
		doFatalError(msg)
		return err
	}
	defer containerClient.Close()

	updateUI := func() {
		// The state is mutable so can't be passed into another goroutine
		// without cloning it first.
		a.eventBus.Send(event.AppStateChangedEvent{State: state.Clone()})
	}
	updateUI()

	var tlsCertPath, tlsKeyPath string
	if a.cfg.Sources.MediaServer.TLS != nil {
		tlsCertPath = a.cfg.Sources.MediaServer.TLS.CertPath
		tlsKeyPath = a.cfg.Sources.MediaServer.TLS.KeyPath
	}

	srv, err := mediaserver.NewActor(ctx, mediaserver.NewActorParams{
		RTMPAddr:        buildNetAddr(a.cfg.Sources.MediaServer.RTMP),
		RTMPSAddr:       buildNetAddr(a.cfg.Sources.MediaServer.RTMPS),
		Host:            a.cfg.Sources.MediaServer.Host,
		TLSCertPath:     tlsCertPath,
		TLSKeyPath:      tlsKeyPath,
		StreamKey:       mediaserver.StreamKey(a.cfg.Sources.MediaServer.StreamKey),
		ContainerClient: containerClient,
		Logger:          a.logger.With("component", "mediaserver"),
	})
	if err != nil {
		err = fmt.Errorf("create mediaserver: %w", err)
		doFatalError(err.Error())
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

	startMediaServerC := make(chan struct{}, 1)
	if a.headless { // disable startup check in headless mode for now
		startMediaServerC <- struct{}{}
	} else {
		if ok, startupErr := doStartupCheck(ctx, containerClient, a.eventBus); startupErr != nil {
			doFatalError(startupErr.Error())
			return startupErr
		} else if ok {
			startMediaServerC <- struct{}{}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-startMediaServerC:
			if err = srv.Start(ctx); err != nil {
				return fmt.Errorf("start mediaserver: %w", err)
			}

			a.eventBus.Send(event.MediaServerStartedEvent{RTMPURL: srv.RTMPURL(), RTMPSURL: srv.RTMPSURL()})
		case <-a.configService.C():
			// No-op, config updates are handled synchronously for now.
		case cmd := <-a.dispatchC:
			if _, err := a.handleCommand(ctx, cmd, state, repl, containerClient, startMediaServerC); errors.Is(err, errExit) {
				return nil
			} else if err != nil {
				return fmt.Errorf("handle command: %w", err)
			}
		case <-uiUpdateT.C:
			updateUI()
		case serverState := <-srv.C():
			a.logger.Debug("Server state received", "state", serverState)

			if serverState.ExitReason != "" {
				doFatalError(serverState.ExitReason)
				return errors.New("media server exited")
			}

			applyServerState(serverState, state)
			updateUI()
		case replState := <-repl.C():
			a.logger.Debug("Replicator state received", "state", replState)
			destErrors := applyReplicatorState(replState, state)

			for _, destError := range destErrors {
				a.eventBus.Send(event.DestinationStreamExitedEvent{Name: destError.name, Err: destError.err})
				repl.StopDestination(destError.url)
			}

			updateUI()
		}
	}
}

type syncCommand struct {
	event.Command

	done chan<- event.Event
}

// Dispatch dispatches a command to be executed synchronously.
func (a *App) Dispatch(cmd event.Command) event.Event {
	ch := make(chan event.Event, 1)
	a.dispatchC <- syncCommand{Command: cmd, done: ch}
	return <-ch
}

// errExit is an error that indicates the app should exit.
var errExit = errors.New("exit")

// handleCommand handles an incoming command. It may return an Event which will
// already have been published to the event bus, but which is returned for the
// benefit of synchronous callers. The event may be nil. It may also publish
// other events to the event bus which are not returned. Currently the only
// error that may be returned is [errExit], which indicates to the main event
// loop that the app should exit.
func (a *App) handleCommand(
	ctx context.Context,
	cmd event.Command,
	state *domain.AppState,
	repl *replicator.Actor,
	containerClient *container.Client,
	startMediaServerC chan struct{},
) (evt event.Event, _ error) {
	a.logger.Debug("Command received", "cmd", cmd.Name())
	defer func() {
		if evt != nil {
			a.eventBus.Send(evt)
		}
		if c, ok := cmd.(syncCommand); ok {
			c.done <- evt
		}
	}()

	switch c := cmd.(type) {
	case event.CommandAddDestination:
		newCfg := a.cfg
		newCfg.Destinations = append(newCfg.Destinations, config.Destination{
			Name: c.DestinationName,
			URL:  c.URL,
		})
		if err := a.configService.SetConfig(newCfg); err != nil {
			a.logger.Error("Add destination failed", "err", err)
			return event.AddDestinationFailedEvent{Err: err}, nil
		}
		a.cfg = newCfg
		a.handleConfigUpdate(state)
		a.eventBus.Send(event.DestinationAddedEvent{URL: c.URL})
	case event.CommandRemoveDestination:
		repl.StopDestination(c.URL) // no-op if not live
		newCfg := a.cfg
		newCfg.Destinations = slices.DeleteFunc(newCfg.Destinations, func(dest config.Destination) bool {
			return dest.URL == c.URL
		})
		if err := a.configService.SetConfig(newCfg); err != nil {
			a.logger.Error("Remove destination failed", "err", err)
			a.eventBus.Send(event.RemoveDestinationFailedEvent{Err: err})
			break
		}
		a.cfg = newCfg
		a.handleConfigUpdate(state)
		a.eventBus.Send(event.DestinationRemovedEvent{URL: c.URL}) //nolint:gosimple
	case event.CommandStartDestination:
		if !state.Source.Live {
			a.eventBus.Send(event.StartDestinationFailedEvent{})
			break
		}

		repl.StartDestination(c.URL)
	case event.CommandStopDestination:
		repl.StopDestination(c.URL)
	case event.CommandCloseOtherInstance:
		if err := closeOtherInstances(ctx, containerClient); err != nil {
			return nil, fmt.Errorf("close other instances: %w", err)
		}

		startMediaServerC <- struct{}{}
	case event.CommandQuit:
		return nil, errExit
	}

	return nil, nil
}

// handleConfigUpdate applies the config to the app state, and sends an AppStateChangedEvent.
func (a *App) handleConfigUpdate(appState *domain.AppState) {
	applyConfig(a.cfg, appState)
	a.eventBus.Send(event.AppStateChangedEvent{State: appState.Clone()})
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

// doStartupCheck performs a startup check to see if there are any existing app
// containers.
//
// It returns a bool if the check is clear. If the bool is false, then
// startup should be paused until the choice selected by the user is received
// via a command.
func doStartupCheck(ctx context.Context, containerClient *container.Client, eventBus *event.Bus) (bool, error) {
	if exists, err := containerClient.ContainerRunning(ctx, container.AllContainers()); err != nil {
		return false, fmt.Errorf("check existing containers: %w", err)
	} else if exists {
		eventBus.Send(event.OtherInstanceDetectedEvent{})

		return false, nil
	}

	return true, nil
}

func closeOtherInstances(ctx context.Context, containerClient *container.Client) error {
	if err := containerClient.RemoveContainers(ctx, container.AllContainers()); err != nil {
		return fmt.Errorf("remove existing containers: %w", err)
	}

	if err := containerClient.RemoveUnusedNetworks(ctx); err != nil {
		return fmt.Errorf("remove unused networks: %w", err)
	}

	return nil
}

// buildNetAddr builds a [mediaserver.OptionalNetAddr] from the config.
func buildNetAddr(src config.RTMPSource) mediaserver.OptionalNetAddr {
	if !src.Enabled {
		return mediaserver.OptionalNetAddr{Enabled: false}
	}

	return mediaserver.OptionalNetAddr{Enabled: true, NetAddr: domain.NetAddr(src.NetAddr)}
}
