package mediaserver

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"

	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
)

// StreamKey is the stream key for the media server, which forms the RTMP path
// component and can be used as a basic form of authentication.
//
// It defaults to "live", in which case the full RTMP URL would be:
// `rtmp://localhost:1935/live`.
type StreamKey string

const (
	defaultFetchIngressStateInterval           = 5 * time.Second                           // default interval to fetch the state of the media server
	defaultAPIPort                             = 9997                                      // default API host port for the media server
	defaultRTMPPort                            = 1935                                      // default RTMP host port for the media server
	defaultChanSize                            = 64                                        // default channel size for asynchronous non-error channels
	imageNameMediaMTX                          = "ghcr.io/rfwatson/mediamtx-alpine:latest" // image name for mediamtx
	defaultStreamKey                 StreamKey = "live"                                    // Default stream key. See [StreamKey].
	componentName                              = "mediaserver"                             // component name, mostly used for Docker labels
	httpClientTimeout                          = time.Second                               // timeout for outgoing HTTP client requests
)

// action is an action to be performed by the actor.
type action func()

// Actor is responsible for managing the media server.
type Actor struct {
	actorC                    chan action
	stateC                    chan domain.Source
	chanSize                  int
	containerClient           *container.Client
	apiPort                   int
	rtmpPort                  int
	streamKey                 StreamKey
	fetchIngressStateInterval time.Duration
	pass                      string // password for the media server
	tlsCert, tlsKey           []byte // TLS cert and key for the media server
	logger                    *slog.Logger
	apiClient                 *http.Client

	// mutable state
	state *domain.Source
}

// NewActorParams contains the parameters for building a new media server
// actor.
type NewActorParams struct {
	APIPort                   int           // defaults to 9997
	RTMPPort                  int           // defaults to 1935
	StreamKey                 StreamKey     // defaults to "live"
	ChanSize                  int           // defaults to 64
	FetchIngressStateInterval time.Duration // defaults to 5 seconds
	ContainerClient           *container.Client
	Logger                    *slog.Logger
}

// NewActor creates a new media server actor.
//
// Callers must consume the state channel exposed via [C].
func NewActor(ctx context.Context, params NewActorParams) (_ *Actor, err error) {
	tlsCert, tlsKey, err := generateTLSCert()
	if err != nil {
		return nil, fmt.Errorf("generate TLS cert: %w", err)
	}
	apiClient, err := buildAPIClient(tlsCert)
	if err != nil {
		return nil, fmt.Errorf("build API client: %w", err)
	}

	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
	return &Actor{
		apiPort:                   cmp.Or(params.APIPort, defaultAPIPort),
		rtmpPort:                  cmp.Or(params.RTMPPort, defaultRTMPPort),
		streamKey:                 cmp.Or(params.StreamKey, defaultStreamKey),
		fetchIngressStateInterval: cmp.Or(params.FetchIngressStateInterval, defaultFetchIngressStateInterval),
		tlsCert:                   tlsCert,
		tlsKey:                    tlsKey,
		pass:                      generatePassword(),
		actorC:                    make(chan action, chanSize),
		state:                     new(domain.Source),
		stateC:                    make(chan domain.Source, chanSize),
		chanSize:                  chanSize,
		containerClient:           params.ContainerClient,
		logger:                    params.Logger,
		apiClient:                 apiClient,
	}, nil
}

func (a *Actor) Start(ctx context.Context) error {
	// Exposed ports are bound to 127.0.0.1 for security.
	// TODO: configurable RTMP bind address
	apiPortSpec := nat.Port("127.0.0.1:" + strconv.Itoa(a.apiPort) + ":9997")
	rtmpPortSpec := nat.Port("127.0.0.1:" + strconv.Itoa(+a.rtmpPort) + ":1935")
	exposedPorts, portBindings, _ := nat.ParsePortSpecs([]string{string(apiPortSpec), string(rtmpPortSpec)})

	// The RTMP URL is passed to the UI via the state.
	// This could be refactored, it's not really stateful data.
	a.state.RTMPURL = a.RTMPURL()

	cfg, err := yaml.Marshal(
		Config{
			LogLevel:        "info",
			LogDestinations: []string{"stdout"},
			AuthMethod:      "internal",
			AuthInternalUsers: []User{
				{
					User: "any",
					IPs:  []string{}, // any IP
					Permissions: []UserPermission{
						{Action: "publish"},
					},
				},
				{
					User: "api",
					Pass: a.pass,
					IPs:  []string{}, // any IP
					Permissions: []UserPermission{
						{Action: "read"},
					},
				},
				{
					User:        "api",
					Pass:        a.pass,
					IPs:         []string{}, // any IP
					Permissions: []UserPermission{{Action: "api"}},
				},
			},
			API:           true,
			APIEncryption: true,
			APIServerCert: "/etc/tls.crt",
			APIServerKey:  "/etc/tls.key",
			Paths: map[string]Path{
				string(a.streamKey): {Source: "publisher"},
			},
		},
	)
	if err != nil { // should never happen
		return fmt.Errorf("marshal config: %w", err)
	}

	containerStateC, errC := a.containerClient.RunContainer(
		ctx,
		container.RunContainerParams{
			Name:     componentName,
			ChanSize: a.chanSize,
			ContainerConfig: &typescontainer.Config{
				Image:    imageNameMediaMTX,
				Hostname: "mediaserver",
				Labels:   map[string]string{container.LabelComponent: componentName},
				Healthcheck: &typescontainer.HealthConfig{
					Test: []string{
						"CMD",
						"curl",
						"--fail",
						"--silent",
						"--cacert", "/etc/tls.crt",
						"--config", "/etc/healthcheckopts.txt",
						a.healthCheckURL(),
					},
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
			CopyFileConfigs: []container.CopyFileConfig{
				{
					Path:    "/mediamtx.yml",
					Payload: bytes.NewReader(cfg),
					Mode:    0600,
				},
				{
					Path:    "/etc/tls.crt",
					Payload: bytes.NewReader(a.tlsCert),
					Mode:    0600,
				},
				{
					Path:    "/etc/tls.key",
					Payload: bytes.NewReader(a.tlsKey),
					Mode:    0600,
				},
				{
					Path:    "/etc/healthcheckopts.txt",
					Payload: bytes.NewReader([]byte(fmt.Sprintf("--user api:%s", a.pass))),
					Mode:    0600,
				},
			},
		},
	)

	go a.actorLoop(ctx, containerStateC, errC)

	return nil
}

// C returns a channel that will receive the current state of the media server.
func (s *Actor) C() <-chan domain.Source {
	return s.stateC
}

// State returns the current state of the media server.
//
// Blocks if the actor is not started yet.
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
		s.containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: componentName}),
	); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	return nil
}

// actorLoop is the main loop of the media server actor. It exits when the
// actor is closed, or the parent context is cancelled.
func (s *Actor) actorLoop(ctx context.Context, containerStateC <-chan domain.Container, errC <-chan error) {
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

			if s.state.Container.Status == domain.ContainerStatusExited {
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
			ingressState, err := fetchIngressState(s.rtmpConnsURL(), s.streamKey, s.apiClient)
			if err != nil {
				s.logger.Error("Error fetching server state", "err", err)
				continue
			}

			var shouldSendState bool
			if ingressState.ready != s.state.Live {
				s.state.Live = ingressState.ready
				s.state.LiveChangedAt = time.Now()
				resetFetchTracksT(time.Second)
				shouldSendState = true
			}
			if ingressState.listeners != s.state.Listeners {
				s.state.Listeners = ingressState.listeners
				shouldSendState = true
			}
			if shouldSendState {
				sendState()
			}
		case <-fetchTracksT.C:
			if !s.state.Live {
				continue
			}

			if tracks, err := fetchTracks(s.pathsURL(), s.streamKey, s.apiClient); err != nil {
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
		case <-ctx.Done():
			return
		}
	}
}

// TODO: refactor to use container.Err?
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

// RTMPURL returns the RTMP URL for the media server, accessible from the host.
func (s *Actor) RTMPURL() string {
	return fmt.Sprintf("rtmp://localhost:%d/%s", s.rtmpPort, s.streamKey)
}

// RTMPInternalURL returns the RTMP URL for the media server, accessible from
// the app network.
func (s *Actor) RTMPInternalURL() string {
	// Container port, not host port:
	return fmt.Sprintf("rtmp://mediaserver:1935/%s?user=api&pass=%s", s.streamKey, s.pass)
}

// rtmpConnsURL returns the URL for fetching RTMP connections, accessible from
// the host.
func (s *Actor) rtmpConnsURL() string {
	return fmt.Sprintf("https://api:%s@localhost:%d/v3/rtmpconns/list", s.pass, s.apiPort)
}

// pathsURL returns the URL for fetching paths, accessible from the host.
func (s *Actor) pathsURL() string {
	return fmt.Sprintf("https://api:%s@localhost:%d/v3/paths/list", s.pass, s.apiPort)
}

// healthCheckURL returns the URL for the health check, accessible from the
// container. It is logged to Docker's events log so must not include
// credentials.
func (s *Actor) healthCheckURL() string {
	return fmt.Sprintf("https://localhost:%d/v3/paths/list", s.apiPort)
}

// shortID returns the first 12 characters of the given container ID.
func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}

// generatePassword securely generates a random password suitable for
// authenticating media server endpoints.
func generatePassword() string {
	const lenBytes = 32
	p := make([]byte, lenBytes)
	_, _ = rand.Read(p)
	return fmt.Sprintf("%x", []byte(p))
}
