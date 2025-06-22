package replicator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	typescontainer "github.com/docker/docker/api/types/container"

	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
)

type action func()

const (
	defaultChanSize = 64           // default channel size for asynchronous non-error channels
	componentName   = "replicator" // component name, mostly used for Docker labels
)

// DefaultImageNameFFMPEG is the default Docker image name for FFmpeg.
const DefaultImageNameFFMPEG = "ghcr.io/jrottenberg/ffmpeg:7.1-scratch"

// State is the state of a single destination from the point of view of the
// replicator.
type State struct {
	URL       string
	Container domain.Container
	Status    domain.DestinationStatus
}

// Actor is responsible for managing the replicator.
type Actor struct {
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	sourceURL       string
	imageName       string
	containerClient *container.Client
	logger          *slog.Logger
	actorC          chan action
	stateC          chan State

	// mutable state

	currURLs  map[string]struct{}
	nextIndex int
}

// StartActorParams contains the parameters for starting a new replicator actor.
type StartActorParams struct {
	SourceURL       string
	ChanSize        int
	ContainerClient *container.Client
	ImageNameFFMPEG string
	Logger          *slog.Logger
}

// StartActor starts a new replicator actor.
//
// The channel exposed by [C] must be consumed by the caller.
func StartActor(ctx context.Context, params StartActorParams) *Actor {
	ctx, cancel := context.WithCancel(ctx)

	actor := &Actor{
		ctx:             ctx,
		cancel:          cancel,
		sourceURL:       params.SourceURL,
		imageName:       cmp.Or(params.ImageNameFFMPEG, DefaultImageNameFFMPEG),
		containerClient: params.ContainerClient,
		logger:          params.Logger,
		actorC:          make(chan action, cmp.Or(params.ChanSize, defaultChanSize)),
		stateC:          make(chan State, cmp.Or(params.ChanSize, defaultChanSize)),
		currURLs:        make(map[string]struct{}),
	}

	go actor.actorLoop()

	return actor
}

// StartDestination starts a destination stream.
func (a *Actor) StartDestination(url string) <-chan State {
	doneC := make(chan State, 1)

	a.actorC <- func() {
		if _, ok := a.currURLs[url]; ok {
			return
		}

		a.nextIndex++
		a.currURLs[url] = struct{}{}

		a.logger.Info("Starting live stream", "url", url)

		containerStateC, errC := a.containerClient.RunContainer(a.ctx, container.RunContainerParams{
			Name: componentName + "-" + strconv.Itoa(a.nextIndex),
			ContainerConfig: &typescontainer.Config{
				Image: a.imageName,
				Cmd: []string{
					"-loglevel", "level+error",
					"-i", a.sourceURL,
					"-c", "copy",
					"-f", "flv",
					url,
				},
				Labels: map[string]string{
					container.LabelComponent: componentName,
					container.LabelURL:       url,
				},
			},
			HostConfig:         &typescontainer.HostConfig{NetworkMode: "default"},
			NetworkCountConfig: container.NetworkCountConfig{Rx: "eth1", Tx: "eth0"},
			Logs:               container.LogConfig{Stderr: true},
			ShouldRestart: func(_ int64, restartCount int, logs [][]byte, runningTime time.Duration) (bool, error) {
				// Try to infer if the container failed to start.
				//
				// For now, we just check if it was running for less than ten seconds.
				if restartCount == 0 && runningTime < 10*time.Second {
					return false, containerStartErrFromLogs(logs)
				}

				// Otherwise, always restart, regardless of the exit code.
				return true, nil
			},
		})

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()

			a.destLoop(url, doneC, containerStateC, errC)
		}()
	}

	return doneC
}

// Grab the first fatal log line, if it exists, or the first error log line,
// from the FFmpeg output.
func containerStartErrFromLogs(logs [][]byte) error {
	var fatalLog, errLog string

	for _, logBytes := range logs {
		log := string(logBytes)
		if strings.HasPrefix(log, "[fatal]") {
			fatalLog = log
			break
		}
	}

	if fatalLog == "" {
		for _, logBytes := range logs {
			log := string(logBytes)
			if strings.HasPrefix(log, "[error]") {
				errLog = log
				break
			}
		}
	}

	return errors.New(cmp.Or(fatalLog, errLog, "container failed to start"))
}

// StopDestination stops a destination stream.
func (a *Actor) StopDestination(url string) {
	a.actorC <- func() {
		if _, ok := a.currURLs[url]; !ok {
			return
		}

		a.logger.Info("Stopping live stream", "url", url)

		if err := a.containerClient.RemoveContainers(
			a.ctx,
			a.containerClient.ContainersWithLabels(map[string]string{
				container.LabelComponent: componentName,
				container.LabelURL:       url,
			}),
		); err != nil {
			// TODO: error handling
			a.logger.Error("Failed to stop live stream", "url", url, "err", err)
		}

		delete(a.currURLs, url)
	}
}

// destLoop is the actor loop for a destination stream.
//
// doneC will be sent a single [State] when the container terminates.
func (a *Actor) destLoop(
	url string,
	doneC chan<- State,
	containerStateC <-chan domain.Container,
	errC <-chan error,
) {
	state := &State{URL: url}
	defer func() { doneC <- *state }()

	sendState := func() { a.stateC <- *state }

	for {
		select {
		case containerState := <-containerStateC:
			state.Container = containerState

			if containerState.Status == domain.ContainerStatusRunning {
				if hasElapsedSince(5*time.Second, containerState.RxSince) {
					state.Status = domain.DestinationStatusLive
				} else {
					state.Status = domain.DestinationStatusStarting
				}
			} else {
				state.Status = domain.DestinationStatusOffAir
			}
			sendState()
		case err, ok := <-errC:
			if !ok {
				sendState()
				return
			}

			if err != nil {
				a.logger.Error("Error from container client", "err", err)
				state.Container.Err = err
				// If the error channel fires, the container state channel will not be
				// updated. We have to clean up here, at least by setting the status to
				// Exited.
				state.Container.Status = domain.ContainerStatusExited
			}
			sendState()
			return
		}
	}
}

// C returns a channel that will receive the current state of the replicator.
// The channel is never closed.
func (a *Actor) C() <-chan State {
	return a.stateC
}

// Close closes the actor.
func (a *Actor) Close() error {
	if err := a.containerClient.RemoveContainers(
		context.Background(),
		a.containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: componentName}),
	); err != nil {
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

// hasElapsedSince returns true if the duration has elapsed since the given
// time. If the provided time is zero, the function returns false.
func hasElapsedSince(d time.Duration, t time.Time) bool {
	if t.IsZero() {
		return false
	}

	return d < time.Since(t)
}
