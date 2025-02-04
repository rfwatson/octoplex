package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"git.netflux.io/rob/termstream/config"
	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/domain"
	"git.netflux.io/rob/termstream/mediaserver"
	"git.netflux.io/rob/termstream/multiplexer"
	"git.netflux.io/rob/termstream/terminal"
	"github.com/docker/docker/client"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, config.FromFile()); err != nil {
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

const uiUpdateInterval = 2 * time.Second

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

	ui, err := terminal.StartActor(ctx, terminal.StartActorParams{Logger: logger.With("component", "ui")})
	if err != nil {
		return fmt.Errorf("start tui: %w", err)
	}
	defer ui.Close()

	updateUI := func() { ui.SetState(*state) }
	updateUI()

	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	containerClient, err := container.NewClient(ctx, apiClient, logger.With("component", "container_client"))
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

			logger.Info("Command received", "cmd", cmd)
			switch c := cmd.(type) {
			case terminal.CommandToggleDestination:
				mp.ToggleDestination(c.URL)
			}
		case <-uiTicker.C:
			// TODO: update UI with current state?
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

func applyMultiplexerState(destination domain.Destination, appState *domain.AppState) {
	for i, dest := range appState.Destinations {
		if dest.URL != destination.URL {
			continue
		}

		appState.Destinations[i] = destination

		break
	}
}

// applyConfig applies the configuration to the app state.
func applyConfig(cfg config.Config, appState *domain.AppState) {
	appState.Destinations = make([]domain.Destination, 0, len(cfg.Destinations))
	for _, dest := range cfg.Destinations {
		appState.Destinations = append(appState.Destinations, domain.Destination{URL: dest.URL})
	}
}
