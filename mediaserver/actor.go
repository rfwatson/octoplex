package mediaserver

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"

	"git.netflux.io/rob/octoplex/container"
	"git.netflux.io/rob/octoplex/domain"
)

const (
	defaultFetchIngressStateInterval = 5 * time.Second                    // default interval to fetch the state of the media server
	defaultAPIPort                   = 9997                               // default API host port for the media server
	defaultRTMPPort                  = 1935                               // default RTMP host port for the media server
	defaultChanSize                  = 64                                 // default channel size for asynchronous non-error channels
	imageNameMediaMTX                = "netfluxio/mediamtx-alpine:latest" // image name for mediamtx
	rtmpPath                         = "live"                             // RTMP path for the media server
	componentName                    = "mediaserver"                      // component name, mostly used for Docker labels
	httpClientTimeout                = time.Second                        // timeout for outgoing HTTP client requests
)

// action is an action to be performed by the actor.
type action func()

// Actor is responsible for managing the media server.
type Actor struct {
	ctx                       context.Context
	cancel                    context.CancelFunc
	actorC                    chan action
	stateC                    chan domain.Source
	containerClient           *container.Client
	apiPort                   int
	rtmpPort                  int
	fetchIngressStateInterval time.Duration
	logger                    *slog.Logger
	httpClient                *http.Client

	// mutable state
	state *domain.Source
}

// StartActorParams contains the parameters for starting a new media server
// actor.
type StartActorParams struct {
	APIPort                   int           // defaults to 9997
	RTMPPort                  int           // defaults to 1935
	ChanSize                  int           // defaults to 64
	FetchIngressStateInterval time.Duration // defaults to 5 seconds
	ContainerClient           *container.Client
	Logger                    *slog.Logger
}

// StartActor starts a new media server actor.
//
// Callers must consume the state channel exposed via [C].
func StartActor(ctx context.Context, params StartActorParams) *Actor {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
	ctx, cancel := context.WithCancel(ctx)

	actor := &Actor{
		ctx:                       ctx,
		cancel:                    cancel,
		apiPort:                   cmp.Or(params.APIPort, defaultAPIPort),
		rtmpPort:                  cmp.Or(params.RTMPPort, defaultRTMPPort),
		fetchIngressStateInterval: cmp.Or(params.FetchIngressStateInterval, defaultFetchIngressStateInterval),
		actorC:                    make(chan action, chanSize),
		state:                     new(domain.Source),
		stateC:                    make(chan domain.Source, chanSize),
		containerClient:           params.ContainerClient,
		logger:                    params.Logger,
		httpClient:                &http.Client{Timeout: httpClientTimeout},
	}

	apiPortSpec := nat.Port(strconv.Itoa(actor.apiPort) + ":9997")
	rtmpPortSpec := nat.Port(strconv.Itoa(actor.rtmpPort) + ":1935")
	exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{string(apiPortSpec), string(rtmpPortSpec)})

	containerStateC, errC := params.ContainerClient.RunContainer(
		ctx,
		container.RunContainerParams{
			Name:     componentName,
			ChanSize: chanSize,
			ContainerConfig: &typescontainer.Config{
				Image:    imageNameMediaMTX,
				Hostname: "mediaserver",
				Env: []string{
					"MTX_LOGLEVEL=info",
					"MTX_API=yes",
				},
				Labels: map[string]string{
					"component": componentName,
				},
				Healthcheck: &typescontainer.HealthConfig{
					Test:          []string{"CMD", "curl", "-f", "http://localhost:9997/v3/paths/list"},
					Interval:      time.Second * 10,
					StartPeriod:   time.Second * 2,
					StartInterval: time.Second * 2,
					Retries:       2,
				},
				ExposedPorts: exposedPorts,
			},
			HostConfig: &typescontainer.HostConfig{
				NetworkMode:  "default",
				PortBindings: portBindings,
			},
			NetworkCountConfig: container.NetworkCountConfig{Rx: "eth0", Tx: "eth1"},
		},
	)

	actor.state.RTMPURL = actor.rtmpURL()
	actor.state.RTMPInternalURL = actor.rtmpInternalURL()

	go actor.actorLoop(containerStateC, errC)

	return actor
}

// C returns a channel that will receive the current state of the media server.
func (s *Actor) C() <-chan domain.Source {
	return s.stateC
}

// State returns the current state of the media server.
func (s *Actor) State() domain.Source {
	resultChan := make(chan domain.Source)
	s.actorC <- func() {
		resultChan <- *s.state
	}
	return <-resultChan
}

// Close closes the media server actor.
func (s *Actor) Close() error {
	if err := s.containerClient.RemoveContainers(
		context.Background(),
		s.containerClient.ContainersWithLabels(map[string]string{"component": componentName}),
	); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	s.cancel()

	return nil
}

// actorLoop is the main loop of the media server actor. It exits when the
// actor is closed, or the parent context is cancelled.
func (s *Actor) actorLoop(containerStateC <-chan domain.Container, errC <-chan error) {
	fetchStateT := time.NewTicker(s.fetchIngressStateInterval)
	defer fetchStateT.Stop()

	// fetchTracksT is used to signal that tracks should be fetched from the
	// media server, after the stream goes on-air. A short delay is needed due to
	// workaround a race condition in the media server.
	var fetchTracksT *time.Timer
	resetFetchTracksT := func(d time.Duration) { fetchTracksT = time.NewTimer(d) }
	resetFetchTracksT(time.Second)
	fetchTracksT.Stop()

	sendState := func() { s.stateC <- *s.state }

	for {
		select {
		case containerState := <-containerStateC:
			s.state.Container = containerState

			if s.state.Container.State == "exited" {
				fetchStateT.Stop()
				s.handleContainerExit(nil)
			}

			sendState()

			continue
		case err, ok := <-errC:
			if !ok {
				// The loop continues until the actor is closed.
				// Avoid receiving duplicate close signals.
				errC = nil
				continue
			}

			if err != nil {
				s.logger.Error("Error from container client", "err", err, "id", shortID(s.state.Container.ID))
			}

			fetchStateT.Stop()
			s.handleContainerExit(err)

			sendState()
		case <-fetchStateT.C:
			ingressState, err := fetchIngressState(s.rtmpConnsURL(), s.httpClient)
			if err != nil {
				s.logger.Error("Error fetching server state", "err", err)
				continue
			}
			if ingressState.ready != s.state.Live || ingressState.listeners != s.state.Listeners {
				s.state.Live = ingressState.ready
				s.state.Listeners = ingressState.listeners
				resetFetchTracksT(time.Second)
				sendState()
			}
		case <-fetchTracksT.C:
			if !s.state.Live {
				continue
			}

			if tracks, err := fetchTracks(s.pathsURL(), s.httpClient); err != nil {
				s.logger.Error("Error fetching tracks", "err", err)
				resetFetchTracksT(3 * time.Second)
			} else if len(tracks) == 0 {
				resetFetchTracksT(time.Second)
			} else {
				s.state.Tracks = tracks
				sendState()
			}
		case action, ok := <-s.actorC:
			if !ok {
				continue
			}
			action()
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Actor) handleContainerExit(err error) {
	if s.state.Container.ExitCode != nil {
		s.state.ExitReason = fmt.Sprintf("Server process exited with code %d.", *s.state.Container.ExitCode)
	} else {
		s.state.ExitReason = "Server process exited unexpectedly."
	}
	if err != nil {
		s.state.ExitReason += "\n\n" + err.Error()
	}

	s.state.Live = false
}

// rtmpURL returns the RTMP URL for the media server, accessible from the host.
func (s *Actor) rtmpURL() string {
	return fmt.Sprintf("rtmp://localhost:%d/%s", s.rtmpPort, rtmpPath)
}

// rtmpInternalURL returns the RTMP URL for the media server, accessible from
// the app network.
func (s *Actor) rtmpInternalURL() string {
	// Container port, not host port:
	return fmt.Sprintf("rtmp://mediaserver:1935/%s", rtmpPath)
}

// rtmpConnsURL returns the URL for fetching RTMP connections, accessible from
// the host.
func (s *Actor) rtmpConnsURL() string {
	return fmt.Sprintf("http://localhost:%d/v3/rtmpconns/list", s.apiPort)
}

// pathsURL returns the URL for fetching paths, accessible from the host.
func (s *Actor) pathsURL() string {
	return fmt.Sprintf("http://localhost:%d/v3/paths/list", s.apiPort)
}

// shortID returns the first 12 characters of the given container ID.
func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}
