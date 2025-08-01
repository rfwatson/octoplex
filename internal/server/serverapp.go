// Package server implements the main application logic for Octoplex server.
package server

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"

	"time"

	"connectrpc.com/connect"
	connectcors "connectrpc.com/cors"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	connectpb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1/internalapiv1connect"
	"git.netflux.io/rob/octoplex/internal/mediaserver"
	"git.netflux.io/rob/octoplex/internal/replicator"
	"git.netflux.io/rob/octoplex/internal/store"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/rs/cors"
)

// command is a wrapper around an event.Command that includes
// additional data required to dispatch it.
type command struct {
	event.Command

	clientID event.ClientID   // may be zero value, if the command is synchronous
	doneC    chan event.Event // may be nil, if the command is asynchronous
}

// App is an instance of the app.
type App struct {
	cfg             config.Config
	credentialsMode CredentialsMode // CredentialsMode indicates whether the API and web credentials are enabled or disabled.
	store           *store.FileStore
	tokenStore      *store.TokenStore
	eventBus        *event.Bus
	dispatchC       chan command
	dockerClient    container.DockerClient
	listenerTLSFunc func() (net.Listener, error)
	keyPairs        domain.KeyPairs
	waitForClient   bool
	logger          *slog.Logger
}

// Params holds the parameters for running the application.
type Params struct {
	Config          config.Config
	Store           *store.FileStore
	TokenStore      *store.TokenStore
	DockerClient    container.DockerClient
	ListenerTLSFunc func() (net.Listener, error) // ListenerTLSFunc overrides the configured TLS listen address. May be nil.
	ChanSize        int
	WaitForClient   bool
	Logger          *slog.Logger
}

// defaultChanSize is the default size of the dispatch channel.
const defaultChanSize = 64

var (
	// ErrOtherInstanceDetected is returned when another instance of the app is
	// detected on startup.
	ErrOtherInstanceDetected = errors.New("another instance is currently running")

	// ErrAuthenticationCannotBeDisabled is returned when it is not permitted for
	// authentication mode to be "none".
	ErrAuthenticationCannotBeDisabled = errors.New("authentication cannot be disabled")
)

// New creates a new application instance.
func New(params Params) (*App, error) {
	cfg := params.Config

	credentialsMode, err := initCredentials(cfg, params.TokenStore, params.Logger)
	if err != nil {
		return nil, fmt.Errorf("build credentials: %w", err)
	}

	keyPairs, err := generateKeyPairs(cfg.ServerURL.Hostname, cfg.TLS, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("build key pairs: %w", err)
	}

	listenerTLSFunc := params.ListenerTLSFunc
	if listenerTLSFunc == nil && cfg.ListenAddrs.TLS != "" {
		listenerTLSFunc = Listener(cfg.ListenAddrs.TLS)
	}

	return &App{
		cfg:             cfg,
		credentialsMode: credentialsMode,
		store:           params.Store,
		tokenStore:      params.TokenStore,
		eventBus:        event.NewBus(params.Logger.With("component", "event_bus")),
		dispatchC:       make(chan command, cmp.Or(params.ChanSize, defaultChanSize)),
		dockerClient:    params.DockerClient,
		listenerTLSFunc: listenerTLSFunc,
		keyPairs:        keyPairs,
		waitForClient:   params.WaitForClient,
		logger:          params.Logger,
	}, nil
}

// Run starts the application, and blocks until it exits.
func (a *App) Run(ctx context.Context) error {
	// state is the current state of the application, as reflected in the UI.
	state := new(domain.AppState)
	applyPersistentState(state, a.store.Get())

	// Ensure there is at least one active source.
	if !a.cfg.Sources.MediaServer.RTMP.Enabled && !a.cfg.Sources.MediaServer.RTMPS.Enabled {
		return errors.New("config: either sources.mediaServer.rtmp.enabled or sources.mediaServer.rtmps.enabled must be set")
	}

	internalAPI := newServer(a.DispatchSync, a.DispatchAsync, a.eventBus, a.logger)

	path, handler := connectpb.NewAPIServiceHandler(internalAPI, connect.WithInterceptors(newAuthInterceptor(a.credentialsMode, a.tokenStore, a.logger)))
	mux := http.NewServeMux()
	mux.Handle(path, newCORSMiddleware(a.cfg.ServerURL.BaseURL, handler))

	if a.cfg.Web.Enabled {
		webHandler, err := newWebHandler(a.cfg, internalAPI, a.credentialsMode, a.tokenStore, a.logger)
		if err != nil {
			return fmt.Errorf("create web handler: %w", err)
		}
		mux.Handle("/", webHandler)
	}

	var tlsServer, plainServer *http.Server
	tlsSrvC := make(chan error, 1)
	plainSrvC := make(chan error, 1)

	cert, err := tls.X509KeyPair(a.keyPairs.External().Cert, a.keyPairs.External().Key)
	if err != nil {
		return fmt.Errorf("load TLS cert: %w", err)
	}

	if a.listenerTLSFunc != nil {
		lisTLS, err := a.listenerTLSFunc()
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		defer lisTLS.Close() //nolint:errcheck

		tlsServer = &http.Server{
			Handler: mux,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   config.TLSMinVersion,
				NextProtos:   []string{"h2"},
			},
			ErrorLog: log.New(&logWriter{a.logger}, "HTTPS: ", 0),
		}

		go func() {
			a.logger.Info("Server started", "tls", "true", "listen-addr", lisTLS.Addr().String())
			tlsSrvC <- tlsServer.Serve(tls.NewListener(lisTLS, tlsServer.TLSConfig))
		}()
	}

	if a.cfg.ListenAddrs.Plain != "" {
		plainServer = &http.Server{
			Addr:     a.cfg.ListenAddrs.Plain,
			Handler:  mux,
			ErrorLog: log.New(&logWriter{a.logger}, "HTTP: ", 0),
		}

		go func() {
			a.logger.Info("Server started", "tls", "false", "listen-addr", a.cfg.ListenAddrs.Plain)
			plainSrvC <- plainServer.ListenAndServe()
		}()
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if tlsServer != nil {
			if shutdownErr := tlsServer.Shutdown(shutdownCtx); shutdownErr != nil {
				a.logger.Error("gRPC server shutdown failed", "err", shutdownErr, "tls", "true")
			}
		}
		if plainServer != nil {
			if shutdownErr := plainServer.Shutdown(shutdownCtx); shutdownErr != nil {
				a.logger.Error("gRPC server shutdown failed", "err", shutdownErr, "tls", "false")
			}
		}
	}()

	if a.waitForClient {
		if err = internalAPI.WaitForClient(ctx); err != nil {
			return fmt.Errorf("wait for client: %w", err)
		}
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

	// doFatalError publishes a fatal error to the event bus. It will block until
	// the user acknowledges it if there is 1 or more clients connected to the
	// internal API.
	doFatalError := func(msg string) {
		a.eventBus.Send(event.FatalErrorOccurredEvent{Message: msg})

		if internalAPI.GetClientCount() == 0 {
			return
		}

		emptyUI()
		<-a.dispatchC
	}

	containerClient, err := container.NewClient(
		ctx,
		container.NewParams{
			APIClient: a.dockerClient,
			InDocker:  a.cfg.InDocker,
			Logger:    a.logger.With("component", "container_client"),
		},
	)
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
	defer containerClient.Close() //nolint:errcheck

	sendAppStateChanged := func() {
		// The state is mutable so can't be passed into another goroutine
		// without cloning it first.
		a.eventBus.Send(event.AppStateChangedEvent{State: state.Clone()})
	}
	sendAppStateChanged()

	srv, err := mediaserver.NewActor(ctx, mediaserver.NewActorParams{
		RTMPAddr:        buildNetAddr(a.cfg.Sources.MediaServer.RTMP),
		RTMPSAddr:       buildNetAddr(a.cfg.Sources.MediaServer.RTMPS),
		Host:            a.cfg.ServerURL.Hostname,
		DockerHost:      a.cfg.DockerHost,
		KeyPairs:        a.keyPairs,
		StreamKey:       mediaserver.StreamKey(a.cfg.Sources.MediaServer.StreamKey),
		ImageName:       a.cfg.Sources.MediaServer.ImageName,
		ContainerClient: containerClient,
		InDocker:        a.cfg.InDocker,
		Logger:          a.logger.With("component", "mediaserver"),
	})
	if err != nil {
		err = fmt.Errorf("create mediaserver: %w", err)
		doFatalError(err.Error())
		return err
	}
	defer srv.Close() //nolint:errcheck

	repl := replicator.StartActor(ctx, replicator.StartActorParams{
		SourceURL:       srv.RTMPInternalURL(),
		ImageNameFFMPEG: a.cfg.ImageNameFFMPEG,
		ContainerClient: containerClient,
		Logger:          a.logger.With("component", "replicator"),
	})
	defer repl.Close() //nolint:errcheck

	const uiUpdateInterval = time.Second
	uiUpdateT := time.NewTicker(uiUpdateInterval)
	defer uiUpdateT.Stop()

	startMediaServerC := make(chan struct{}, 1)
	if ok, startupErr := doStartupCheck(ctx, containerClient, a.eventBus); startupErr != nil {
		doFatalError(startupErr.Error())
		return startupErr
	} else if ok {
		startMediaServerC <- struct{}{}
	} else if internalAPI.GetClientCount() == 0 {
		// startup check failed, and there are no clients connected to the API so
		// probably in server-only mode. In this case, we just bail out.
		return ErrOtherInstanceDetected
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case srvErr := <-tlsSrvC:
			if srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
				a.logger.Error("gRPC server exited with error", "err", srvErr, "tls", "true")
			}
			return srvErr
		case srvErr := <-plainSrvC:
			if srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
				a.logger.Error("gRPC server exited with error", "err", srvErr, "tls", "false")
			}
			return srvErr
		case <-startMediaServerC:
			if err = srv.Start(ctx); err != nil {
				return fmt.Errorf("start mediaserver: %w", err)
			}

			sendAppStateChanged()
		case cmd := <-a.dispatchC:
			// TODO: doneC must always be called if non-nil, even if the command is unsuccessful.
			okEvt, errEvt, err := a.handleCommand(ctx, cmd, state, repl, containerClient, startMediaServerC)
			if err != nil {
				if errors.Is(err, errExit) {
					return nil
				} else {
					return fmt.Errorf("handle command: %w", err)
				}
			}

			if errEvt != nil {
				// Command execution failed, send the error to the event bus for
				// this client only.
				if cmd.clientID != "" {
					a.eventBus.SendTo(cmd.clientID, errEvt)
				}
				if cmd.doneC != nil {
					cmd.doneC <- errEvt
				}
				continue
			}

			// TODO: always return a non-nil event from handleCommand
			if okEvt == nil {
				continue
			}

			// Command execution successful, send the event to the event bus for
			// all clients.
			a.eventBus.Send(okEvt)

			if cmd.doneC != nil {
				cmd.doneC <- okEvt
			}
		case <-uiUpdateT.C:
			sendAppStateChanged()
		case serverState := <-srv.C():
			a.logger.Debug("Server state received", "state", serverState)

			if serverState.ExitReason != "" {
				doFatalError(serverState.ExitReason)
				return errors.New("media server exited")
			}

			applyServerState(serverState, state)
			sendAppStateChanged()
		case replState := <-repl.C():
			a.logger.Debug("Replicator state received", "state", replState)
			applyReplicatorState(state, replState)
			sendAppStateChanged()
		}
	}
}

// DispatchSync dispatches a command to be executed synchronously.
func (a *App) DispatchSync(cmd event.Command) (event.Event, error) {
	ch := make(chan event.Event, 1)
	a.dispatchC <- command{
		Command: cmd,
		doneC:   ch,
	}

	return <-ch, nil
}

// DispatchAsync dispatches a command to be executed synchronously.
func (a *App) DispatchAsync(clientID event.ClientID, cmd event.Command) {
	a.dispatchC <- command{Command: cmd, clientID: clientID}
}

// errExit is an error that indicates the server should exit.
var errExit = errors.New("exit")

// handleCommand handles an incoming command.
//
// It returns one of:
//
//   - a successful event, which should be sent to all clients
//   - an error event, which should be sent to the client that sent the command
//   - an error, which is currently only ever [errExit] to indicate that the
//     server should exit cleanly.
func (a *App) handleCommand(
	ctx context.Context,
	cmd command,
	state *domain.AppState,
	repl *replicator.Actor,
	containerClient *container.Client,
	startMediaServerC chan struct{},
) (evt event.Event, errEvt event.Event, err error) {
	a.logger.Debug("Command received in handler", "cmd", cmd.Name())

	switch c := cmd.Command.(type) {
	case event.CommandListDestinations:
		// Listing destinations isn't really an error.
		// Probably errEvt should be renamed, it's just an event only sent to the
		// client which triggered the command.
		return nil, event.DestinationsListedEvent{Destinations: slices.Clone(state.Destinations)}, nil
	case event.CommandAddDestination:
		destinationID := uuid.New()
		newState := a.store.Get()
		newState.Destinations = append(newState.Destinations, store.Destination{
			ID:   destinationID,
			Name: c.DestinationName,
			URL:  c.URL,
		})
		if validationErrors, err := a.store.Set(newState); err != nil {
			a.logger.Error("Add destination failed", "err", err)
			return nil, event.AddDestinationFailedEvent{URL: c.URL, Err: err}, nil
		} else if len(validationErrors) > 0 {
			a.logger.Info("Add destination failed", "validation_errors", validationErrors)
			return nil, event.AddDestinationFailedEvent{URL: c.URL, ValidationErrors: validationErrors}, nil
		}
		a.handlePersistentStateUpdate(state)
		a.logger.Info("Destination added", "id", destinationID, "name", c.DestinationName)
		return event.DestinationAddedEvent{ID: destinationID}, nil, nil
	case event.CommandUpdateDestination:
		if isLive(state, c.ID) {
			// should be caught in the UI, but just in case
			a.logger.Warn("Update destination failed: destination is live", "id", c.ID)
			return nil, event.UpdateDestinationFailedEvent{ID: c.ID, Err: errors.New("destination is live")}, nil
		}

		newState := a.store.Get()
		idx := slices.IndexFunc(newState.Destinations, func(dest store.Destination) bool { return dest.ID == c.ID })
		if idx == -1 {
			a.logger.Warn("Update destination failed: destination not found", "id", c.ID)
			return nil, event.UpdateDestinationFailedEvent{ID: c.ID, Err: fmt.Errorf("destination not found")}, nil
		}

		dest := &newState.Destinations[idx]
		if c.DestinationName.IsPresent() {
			dest.Name = c.DestinationName.Value
		}
		if c.URL.IsPresent() {
			dest.URL = c.URL.Value
		}

		if validationErrors, err := a.store.Set(newState); err != nil {
			a.logger.Error("Update destination failed", "err", err)
			return nil, event.UpdateDestinationFailedEvent{ID: c.ID, Err: err}, nil
		} else if len(validationErrors) > 0 {
			a.logger.Info("Update destination failed", "validation_errors", validationErrors)
			return nil, event.UpdateDestinationFailedEvent{ID: c.ID, ValidationErrors: validationErrors}, nil
		}
		a.handlePersistentStateUpdate(state)
		a.logger.Info("Destination updated", "id", c.ID, "name", dest.Name)
		return event.DestinationUpdatedEvent{ID: c.ID}, nil, nil
	case event.CommandRemoveDestination:
		newState := a.store.Get()

		idx := slices.IndexFunc(newState.Destinations, func(dest store.Destination) bool { return dest.ID == c.ID })
		if idx == -1 {
			a.logger.Warn("Remove destination failed: destination not found", "id", c.ID)
			return nil, event.RemoveDestinationFailedEvent{ID: c.ID, Err: fmt.Errorf("destination not found")}, nil
		}

		dest := state.Destinations[idx]
		if dest.Status == domain.DestinationStatusLive && !c.Force {
			return nil, event.RemoveDestinationFailedEvent{ID: c.ID, Err: errors.New("destination is live")}, nil
		}

		repl.StopDestination(dest.URL) // no-op if not live
		newState.Destinations = slices.Delete(newState.Destinations, idx, idx+1)

		if validationErrors, err := a.store.Set(newState); err != nil {
			a.logger.Error("Remove destination failed", "err", err)
			return nil, event.RemoveDestinationFailedEvent{ID: c.ID, Err: err}, nil
		} else if len(validationErrors) > 0 { // This case shouldn't happen, there is no validation when removing a destination.
			a.logger.Info("Remove destination failed", "validation_errors", validationErrors)
			return nil, event.RemoveDestinationFailedEvent{ID: c.ID, Err: errors.New("validation failed")}, nil
		}
		a.handlePersistentStateUpdate(state)
		a.logger.Info("Destination removed", "id", c.ID, "name", dest.Name)
		return event.DestinationRemovedEvent{ID: c.ID}, nil, nil //nolint:staticcheck
	case event.CommandStartDestination:
		destIndex := slices.IndexFunc(state.Destinations, func(d domain.Destination) bool {
			return d.ID == c.ID
		})
		if destIndex == -1 {
			a.logger.Warn("Start destination failed: destination not found", "id", c.ID)
			return nil, event.StartDestinationFailedEvent{ID: c.ID, Err: fmt.Errorf("destination not found")}, nil
		}

		if !state.Source.Live {
			return nil, event.StartDestinationFailedEvent{ID: c.ID, Err: errors.New("source not live")}, nil
		}

		dest := state.Destinations[destIndex]
		doneC := repl.StartDestination(dest.URL)
		a.logger.Info("Destination started", "id", c.ID, "name", dest.Name)
		go func() {
			select {
			case state := <-doneC:
				if cmd.clientID != "" && state.Container.Err != nil {
					a.eventBus.SendTo(cmd.clientID, event.DestinationStreamExitedEvent{ID: dest.ID, Name: dest.Name, Err: state.Container.Err})
				}

				repl.StopDestination(dest.URL)
			case <-ctx.Done():
			}
		}()

		return event.DestinationStartedEvent{ID: c.ID}, nil, nil //nolint:staticcheck
	case event.CommandStopDestination:
		destIndex := slices.IndexFunc(state.Destinations, func(d domain.Destination) bool {
			return d.ID == c.ID
		})
		if destIndex == -1 {
			a.logger.Warn("Start destination failed: destination not found", "id", c.ID)
			return nil, event.StopDestinationFailedEvent{ID: c.ID, Err: fmt.Errorf("destination not found")}, nil
		}

		repl.StopDestination(state.Destinations[destIndex].URL)
		a.logger.Info("Destination stopped", "id", c.ID, "name", state.Destinations[destIndex].Name)
		return event.DestinationStoppedEvent{ID: c.ID}, nil, nil //nolint:staticcheck
	case event.CommandCloseOtherInstance:
		if err := closeOtherInstances(ctx, containerClient); err != nil {
			return nil, nil, fmt.Errorf("close other instances: %w", err) // TODO: improve error handling
		}

		startMediaServerC <- struct{}{}
		return nil, nil, nil //nolint:staticcheck
	case event.CommandKillServer:
		return nil, nil, errExit
	default:
		return nil, nil, fmt.Errorf("unknown command: %T", cmd.Command)
	}
}

func isLive(state *domain.AppState, destinationID uuid.UUID) bool {
	for _, dest := range state.Destinations {
		if dest.ID == destinationID {
			return dest.Status == domain.DestinationStatusLive
		}
	}

	return false
}

// handlePersistentStateUpdate applies the persistent state to the app state,
// amd sends a AppStateChangedEvent.
func (a *App) handlePersistentStateUpdate(appState *domain.AppState) {
	applyPersistentState(appState, a.store.Get())
	a.eventBus.Send(event.AppStateChangedEvent{State: appState.Clone()})
}

// applyServerState applies the current server state to the app state.
func applyServerState(serverState domain.Source, appState *domain.AppState) {
	appState.Source = serverState
}

// applyReplicatorState applies the current replicator state for a single
// destination to the app state.
func applyReplicatorState(appState *domain.AppState, replState replicator.State) {
	index := slices.IndexFunc(appState.Destinations, func(dest domain.Destination) bool {
		return dest.URL == replState.URL
	})

	if index == -1 {
		return
	}

	dest := &appState.Destinations[index]
	dest.Container = replState.Container
	dest.Status = replState.Status
}

// applyPersistentState applies the persistent state to the runtime state. For
// now we only set the destinations, which is the only attribute that changes
// at runtime.
func applyPersistentState(appState *domain.AppState, state store.State) {
	appState.Destinations = destinationsFromStore(state.Destinations)
}

func destinationsFromStore(inDestinations []store.Destination) []domain.Destination {
	if len(inDestinations) == 0 {
		return nil
	}

	destinations := make([]domain.Destination, 0, len(inDestinations))
	for _, inDest := range inDestinations {
		destinations = append(destinations, domain.Destination{
			ID:   inDest.ID,
			Name: inDest.Name,
			URL:  inDest.URL,
		})
	}

	return destinations
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

// Stop stops all containers and networks created by any instance of the app.
func (a *App) Stop(ctx context.Context) error {
	containerClient, err := container.NewClient(ctx, container.NewParams{APIClient: a.dockerClient, Logger: a.logger.With("component", "container_client")})
	if err != nil {
		return fmt.Errorf("create container client: %w", err)
	}
	defer containerClient.Close() //nolint:errcheck

	return closeOtherInstances(ctx, containerClient)
}

type logWriter struct {
	logger *slog.Logger
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.logger.Info("H2 error log", "msg", string(p))
	return len(p), nil
}

func newCORSMiddleware(domain string, next http.Handler) http.Handler {
	corsOptions := cors.Options{
		AllowedOrigins: []string{strings.TrimSuffix(domain, "/")},
		AllowedMethods: connectcors.AllowedMethods(),
		AllowedHeaders: connectcors.AllowedHeaders(),
		ExposedHeaders: connectcors.ExposedHeaders(),
	}

	return cors.New(corsOptions).Handler(next)
}
