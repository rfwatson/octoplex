package multiplexer

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	typescontainer "github.com/docker/docker/api/types/container"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/domain"
)

type action func()

const (
	defaultChanSize = 64                                       // default channel size for asynchronous non-error channels
	componentName   = "multiplexer"                            // component name, mostly used for Docker labels
	imageNameFFMPEG = "ghcr.io/jrottenberg/ffmpeg:7.1-scratch" // image name for ffmpeg
)

// Actor is responsible for managing the multiplexer.
type Actor struct {
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	sourceURL       string
	containerClient *container.Client
	logger          *slog.Logger
	actorC          chan action
	stateC          chan domain.Destination

	// mutable state
	currURLs  map[string]struct{}
	nextIndex int
}

// NewActorParams contains the parameters for starting a new multiplexer actor.
type NewActorParams struct {
	SourceURL       string
	ChanSize        int
	ContainerClient *container.Client
	Logger          *slog.Logger
}

// NewActor starts a new multiplexer actor.
//
// The channel exposed by [C] must be consumed by the caller.
func NewActor(ctx context.Context, params NewActorParams) *Actor {
	ctx, cancel := context.WithCancel(ctx)

	actor := &Actor{
		ctx:             ctx,
		cancel:          cancel,
		sourceURL:       params.SourceURL,
		containerClient: params.ContainerClient,
		logger:          params.Logger,
		actorC:          make(chan action, cmp.Or(params.ChanSize, defaultChanSize)),
		stateC:          make(chan domain.Destination, cmp.Or(params.ChanSize, defaultChanSize)),
		currURLs:        make(map[string]struct{}),
	}

	go actor.actorLoop()

	return actor
}

// ToggleDestination toggles the destination stream between on and off.
func (a *Actor) ToggleDestination(url string) {
	a.actorC <- func() {
		labels := map[string]string{"component": componentName, "url": url}

		if _, ok := a.currURLs[url]; ok {
			a.logger.Info("Stopping live stream", "url", url)

			if err := a.containerClient.RemoveContainers(a.ctx, labels); err != nil {
				// TODO: error handling
				a.logger.Error("Failed to stop live stream", "url", url, "error", err)
			}

			delete(a.currURLs, url)
			return
		}

		a.logger.Info("Starting live stream", "url", url)

		containerStateC, errC := a.containerClient.RunContainer(a.ctx, container.RunContainerParams{
			Name: componentName + "-" + strconv.Itoa(a.nextIndex),
			ContainerConfig: &typescontainer.Config{
				Image: imageNameFFMPEG,
				Cmd: []string{
					"-i", a.sourceURL,
					"-c", "copy",
					"-f", "flv",
					url,
				},
				Labels: labels,
			},
			HostConfig: &typescontainer.HostConfig{
				NetworkMode: "host",
			},
		})

		a.nextIndex++
		a.currURLs[url] = struct{}{}

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()

			a.destLoop(url, containerStateC, errC)
		}()
	}
}

// destLoop is the actor loop for a destination stream.
func (a *Actor) destLoop(url string, containerStateC <-chan domain.Container, errC <-chan error) {
	defer func() {
		a.actorC <- func() {
			delete(a.currURLs, url)
		}
	}()

	state := &domain.Destination{URL: url}
	sendState := func() { a.stateC <- *state }

	for {
		select {
		case containerState := <-containerStateC:
			state.Container = containerState
			state.Live = containerState.State == "running"
			sendState()
		case err := <-errC:
			// TODO: error handling
			if err != nil {
				a.logger.Error("Error from container client", "error", err)
			}
			return
		}
	}
}

// C returns a channel that will receive the current state of the multiplexer.
// The channel is never closed.
func (a *Actor) C() <-chan domain.Destination {
	return a.stateC
}

// Close closes the actor.
func (a *Actor) Close() error {
	if err := a.containerClient.RemoveContainers(context.Background(), map[string]string{"component": componentName}); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	a.wg.Wait()

	close(a.actorC)

	return nil
}

// actorLoop is the main actor loop.
func (a *Actor) actorLoop() {
	for act := range a.actorC {
		act()
	}
}
