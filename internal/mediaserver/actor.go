package mediaserver

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
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
	defaultUpdateStateInterval           = 5 * time.Second                           // default interval to update the state of the media server
	defaultAPIPort                       = 9997                                      // default API host port for the media server
	defaultRTMPIP                        = "127.0.0.1"                               // default RTMP host IP, bound to localhost for security
	defaultRTMPPort                      = 1935                                      // default RTMP host port for the media server
	defaultRTMPSPort                     = 1936                                      // default RTMPS host port for the media server
	defaultHost                          = "localhost"                               // default mediaserver host name
	defaultChanSize                      = 64                                        // default channel size for asynchronous non-error channels
	imageNameMediaMTX                    = "ghcr.io/rfwatson/mediamtx-alpine:latest" // image name for mediamtx
	defaultStreamKey           StreamKey = "live"                                    // Default stream key. See [StreamKey].
	componentName                        = "mediaserver"                             // component name, mostly used for Docker labels
	httpClientTimeout                    = time.Second                               // timeout for outgoing HTTP client requests
)

// action is an action to be performed by the actor.
type action func()

// Actor is responsible for managing the media server.
type Actor struct {
	actorC              chan action
	stateC              chan domain.Source
	chanSize            int
	containerClient     *container.Client
	rtmpAddr            domain.NetAddr
	rtmpsAddr           domain.NetAddr
	apiPort             int
	host                string
	streamKey           StreamKey
	updateStateInterval time.Duration
	pass                string // password for the media server
	tlsCert, tlsKey     []byte // TLS cert and key for the media server
	logger              *slog.Logger
	apiClient           *http.Client

	// mutable state
	state *domain.Source
}

// NewActorParams contains the parameters for building a new media server
// actor.
type NewActorParams struct {
	RTMPAddr            OptionalNetAddr // defaults to disabled, or 127.0.0.1:1935
	RTMPSAddr           OptionalNetAddr // defaults to disabled, or 127.0.0.1:1936
	APIPort             int             // defaults to 9997
	Host                string          // defaults to "localhost"
	StreamKey           StreamKey       // defaults to "live"
	ChanSize            int             // defaults to 64
	UpdateStateInterval time.Duration   // defaults to 5 seconds
	ContainerClient     *container.Client
	Logger              *slog.Logger
}

// OptionalNetAddr is a wrapper around domain.NetAddr that indicates whether it
// is enabled or not.
type OptionalNetAddr struct {
	domain.NetAddr

	Enabled bool
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
		rtmpAddr:            toRTMPAddr(params.RTMPAddr, defaultRTMPPort),
		rtmpsAddr:           toRTMPAddr(params.RTMPSAddr, defaultRTMPSPort),
		apiPort:             cmp.Or(params.APIPort, defaultAPIPort),
		host:                cmp.Or(params.Host, defaultHost),
		streamKey:           cmp.Or(params.StreamKey, defaultStreamKey),
		updateStateInterval: cmp.Or(params.UpdateStateInterval, defaultUpdateStateInterval),
		tlsCert:             tlsCert,
		tlsKey:              tlsKey,
		pass:                generatePassword(),
		actorC:              make(chan action, chanSize),
		state:               new(domain.Source),
		stateC:              make(chan domain.Source, chanSize),
		chanSize:            chanSize,
		containerClient:     params.ContainerClient,
		logger:              params.Logger,
		apiClient:           apiClient,
	}, nil
}

func (a *Actor) Start(ctx context.Context) error {
	var portSpecs []string
	portSpecs = append(portSpecs, fmt.Sprintf("127.0.0.1:%d:9997", a.apiPort))
	if !a.rtmpAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtmpAddr.IP, a.rtmpAddr.Port, 1935))
	}
	if !a.rtmpsAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtmpsAddr.IP, a.rtmpsAddr.Port, 1936))
	}
	exposedPorts, portBindings, err := nat.ParsePortSpecs(portSpecs)
	if err != nil {
		return fmt.Errorf("parse port specs: %w", err)
	}

	cfg, err := a.buildServerConfig()
	if err != nil {
		return fmt.Errorf("build server config: %w", err)
	}

	args := []any{"host", a.host}
	if a.rtmpAddr.IsZero() {
		args = append(args, "rtmp.enabled", false)
	} else {
		args = append(args, "rtmp.enabled", true, "rtmp.bind_addr", a.rtmpAddr.IP, "rtmp.bind_port", a.rtmpAddr.Port)
	}
	if a.rtmpsAddr.IsZero() {
		args = append(args, "rtmps.enabled", false)
	} else {
		args = append(args, "rtmps.enabled", true, "rtmps.bind_addr", a.rtmpsAddr.IP, "rtmps.bind_port", a.rtmpsAddr.Port)
	}
	a.logger.Info("Starting media server", args...)

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
			Logs:               container.LogConfig{Stdout: true},
			CopyFiles: []container.CopyFileConfig{
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

func (a *Actor) buildServerConfig() ([]byte, error) {
	// NOTE: Regardless of the user configuration (which mostly affects exposed
	// ports and UI rendering) plain RTMP must be enabled at the container level,
	// for internal connections.
	var encryptionString string
	if a.rtmpsAddr.IsZero() {
		encryptionString = "no"
	} else {
		encryptionString = "optional"
	}

	return yaml.Marshal(
		Config{
			LogLevel:        "debug",
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
			RTMP:           true,
			RTMPEncryption: encryptionString,
			RTMPAddress:    ":1935",
			RTMPSAddress:   ":1936",
			RTMPServerCert: "/etc/tls.crt", // TODO: custom certs
			RTMPServerKey:  "/etc/tls.key", // TODO: custom certs
			API:            true,
			APIEncryption:  true,
			APIServerCert:  "/etc/tls.crt",
			APIServerKey:   "/etc/tls.key",
			Paths: map[string]Path{
				string(a.streamKey): {Source: "publisher"},
			},
		},
	)
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
	updateStateT := time.NewTicker(s.updateStateInterval)
	defer updateStateT.Stop()

	sendState := func() { s.stateC <- *s.state }

	for {
		select {
		case containerState := <-containerStateC:
			s.state.Container = containerState

			if s.state.Container.Status == domain.ContainerStatusExited {
				updateStateT.Stop()
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

			updateStateT.Stop()
			s.handleContainerExit(err)

			sendState()
		case <-updateStateT.C:
			path, err := fetchPath(s.pathURL(string(s.streamKey)), s.apiClient)
			if err != nil {
				s.logger.Error("Error fetching path", "err", err)
				continue
			}

			if path.Ready != s.state.Live {
				s.state.Live = path.Ready
				s.state.LiveChangedAt = time.Now()
				s.state.Tracks = path.Tracks
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
	if s.rtmpAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtmp://%s:%d/%s", s.host, s.rtmpAddr.Port, s.streamKey)
}

// RTMPSURL returns the RTMPS URL for the media server, accessible from the host.
func (s *Actor) RTMPSURL() string {
	if s.rtmpsAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtmps://%s:%d/%s", s.host, s.rtmpsAddr.Port, s.streamKey)
}

// RTMPInternalURL returns the RTMP URL for the media server, accessible from
// the app network.
func (s *Actor) RTMPInternalURL() string {
	// Container port, not host port:
	return fmt.Sprintf("rtmp://mediaserver:1935/%s?user=api&pass=%s", s.streamKey, s.pass)
}

// pathURL returns the URL for fetching a path, accessible from the host.
func (s *Actor) pathURL(path string) string {
	return fmt.Sprintf("https://api:%s@localhost:%d/v3/paths/get/%s", s.pass, s.apiPort, path)
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

// toRTMPAddr builds a domain.NetAddr from an OptionalNetAddr, with default
// values set to RTMP default bind config if needed. If the OptionalNetAddr is
// not enabled, a zero value is returned.
func toRTMPAddr(a OptionalNetAddr, defaultPort int) domain.NetAddr {
	if !a.Enabled {
		return domain.NetAddr{}
	}

	return domain.NetAddr{
		IP:   cmp.Or(a.IP, defaultRTMPIP),
		Port: cmp.Or(a.Port, defaultPort),
	}
}
