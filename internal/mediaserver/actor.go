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
	defaultUpdateStateInterval           = 5 * time.Second         // default interval to update the state of the media server
	defaultAPIPort                       = 9997                    // default API host port for the media server
	defaultRTMPIP                        = "127.0.0.1"             // default RTMP host IP, bound to localhost for security
	defaultRTMPPort                      = 1935                    // default RTMP host port for the media server
	defaultRTMPSPort                     = 1936                    // default RTMPS host port for the media server
	defaultHost                          = "localhost"             // default mediaserver host name
	defaultChanSize                      = 64                      // default channel size for asynchronous non-error channels
	defaultStreamKey           StreamKey = "live"                  // Default stream key. See [StreamKey].
	componentName                        = "mediaserver"           // component name, used for Docker hostname and labels
	httpClientTimeout                    = time.Second             // timeout for outgoing HTTP client requests
	configPath                           = "/mediamtx.yml"         // path to the media server config file
	tlsInternalCertPath                  = "/etc/tls-internal.crt" // path to the internal TLS cert
	tlsInternalKeyPath                   = "/etc/tls-internal.key" // path to the internal TLS key
	tlsCertPath                          = "/etc/tls.crt"          // path to the custom TLS cert
	tlsKeyPath                           = "/etc/tls.key"          // path to the custom TLS key
)

// DefaultImageNameMediaMTX is the default Docker image name for
// the MediaMTX server.
const DefaultImageNameMediaMTX = "ghcr.io/rfwatson/mediamtx-alpine:latest"

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
	imageName           string
	updateStateInterval time.Duration
	pass                string // password for the media server
	keyPairs            domain.KeyPairs
	inDocker            bool
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
	ImageName           string          // defaults to "ghcr.io/rfwatson/mediamtx-alpine:latest"
	ChanSize            int             // defaults to 64
	UpdateStateInterval time.Duration   // defaults to 5 seconds
	KeyPairs            domain.KeyPairs
	ContainerClient     *container.Client
	InDocker            bool
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
	// TODO: custom cert for API?
	apiClient, err := buildAPIClient(params.KeyPairs.Internal.Cert)
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
		imageName:           cmp.Or(params.ImageName, DefaultImageNameMediaMTX),
		updateStateInterval: cmp.Or(params.UpdateStateInterval, defaultUpdateStateInterval),
		keyPairs:            params.KeyPairs,
		pass:                generatePassword(),
		actorC:              make(chan action, chanSize),
		state:               new(domain.Source),
		stateC:              make(chan domain.Source, chanSize),
		chanSize:            chanSize,
		containerClient:     params.ContainerClient,
		inDocker:            params.InDocker,
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

	// RTMP URLs are passed in state mostly to ensure that late-joining clients
	// can receive them. It might be nice to clean this up, the URLs don't change
	// so aren't really stateful data and don't need to be sent in every update.
	a.state.RTMPURL = a.RTMPURL()
	a.state.RTMPSURL = a.RTMPSURL()

	cfg, err := a.buildServerConfig()
	if err != nil {
		return fmt.Errorf("build server config: %w", err)
	}

	copyFiles := []container.CopyFileConfig{
		{
			Path:    configPath,
			Payload: bytes.NewReader(cfg),
			Mode:    0600,
		},
		{
			Path:    tlsInternalCertPath,
			Payload: bytes.NewReader(a.keyPairs.Internal.Cert),
			Mode:    0600,
		},
		{
			Path:    tlsInternalKeyPath,
			Payload: bytes.NewReader(a.keyPairs.Internal.Key),
			Mode:    0600,
		},
		{
			Path:    "/etc/healthcheckopts.txt",
			Payload: bytes.NewReader([]byte(fmt.Sprintf("--user api:%s", a.pass))),
			Mode:    0600,
		},
	}

	if !a.keyPairs.Custom.IsZero() {
		copyFiles = append(
			copyFiles,
			container.CopyFileConfig{
				Path:    tlsCertPath,
				Payload: bytes.NewReader(a.keyPairs.Custom.Cert),
				Mode:    0600,
			},
			container.CopyFileConfig{
				Path:    tlsKeyPath,
				Payload: bytes.NewReader(a.keyPairs.Custom.Key),
				Mode:    0600,
			},
		)
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
				Image:    a.imageName,
				Hostname: componentName,
				Labels:   map[string]string{container.LabelComponent: componentName},
				Healthcheck: &typescontainer.HealthConfig{
					Test: []string{
						"CMD",
						"curl",
						"--fail",
						"--silent",
						"--cacert", "/etc/tls-internal.crt",
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
			Logs:      container.LogConfig{Stdout: true},
			CopyFiles: copyFiles,
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

	var certPath, keyPath string
	if a.keyPairs.Custom.IsZero() {
		certPath = tlsInternalCertPath
		keyPath = tlsInternalKeyPath
	} else {
		certPath = tlsCertPath
		keyPath = tlsKeyPath
	}

	return yaml.Marshal(
		Config{
			LogLevel:        "info",
			LogDestinations: []string{"stdout"},
			AuthMethod:      "internal",
			AuthInternalUsers: []User{
				{
					User: "any",
					IPs:  []string{}, // allow any IP
					Permissions: []UserPermission{
						{Action: "publish"},
					},
				},
				{
					User: "api",
					Pass: a.pass,
					IPs:  []string{}, // allow any IP
					Permissions: []UserPermission{
						{Action: "read"},
					},
				},
				{
					User:        "api",
					Pass:        a.pass,
					IPs:         []string{}, // allow any IP
					Permissions: []UserPermission{{Action: "api"}},
				},
			},
			RTMP:           true,
			RTMPEncryption: encryptionString,
			RTMPAddress:    ":1935",
			RTMPSAddress:   ":1936",
			RTMPServerCert: certPath,
			RTMPServerKey:  keyPath,
			API:            true,
			APIEncryption:  true,
			APIServerCert:  tlsInternalCertPath,
			APIServerKey:   tlsInternalKeyPath,
			APIAddress:     ":9997",
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
	updateStateT.Stop() // only start when the container starts
	defer updateStateT.Stop()

	sendState := func() { s.stateC <- *s.state }

	for {
		select {
		case containerState := <-containerStateC:
			if containerState.Status == domain.ContainerStatusRunning && s.state.Container.Status != domain.ContainerStatusRunning {
				// The container has moved into running state.
				// Start polling the API.
				updateStateT.Reset(s.updateStateInterval)
			}

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

// pathURL returns the URL for fetching a path.
//
// When running in Docker, the URL must only be accessible from the container,
// via the container hostname (i.e. mediaserver). In this case, it does not
// need to be exposed publicly.
//
// When running outside Docker, it's assumed that Octoplex is running on the
// Docker host, so the API should be accessible over localhost.
func (s *Actor) pathURL(path string) string {
	var hostname string
	if s.inDocker {
		hostname = componentName
	} else {
		hostname = "localhost"
	}

	return fmt.Sprintf("https://api:%s@%s:%d/v3/paths/get/%s", s.pass, hostname, s.apiPort, path)
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
