package mediaserver

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"

	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/optional"
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
	defaultRTSPPort                      = 8554                    // default RTSP host port for the media server
	defaultRTSPSPort                     = 8332                    // default RTSPS host port for the media server
	defaultHost                          = "localhost"             // default mediaserver host name
	defaultDockerHost                    = "localhost"             // default hostname to connect to Docker containers
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

// ContainerClient isolates *container.Client.
type ContainerClient interface {
	ContainersWithLabels(map[string]string) container.LabelOptions
	HasDockerNetwork() bool
	RemoveContainers(context.Context, container.LabelOptions) error
	RunContainer(context.Context, container.RunContainerParams) (<-chan domain.Container, <-chan error)
}

// Actor is responsible for managing the media server.
type Actor struct {
	actorC              chan action
	stateC              chan domain.Source
	chanSize            int
	containerClient     ContainerClient
	rtmpAddr            domain.NetAddr
	rtmpsAddr           domain.NetAddr
	rtspAddr            domain.NetAddr
	rtspsAddr           domain.NetAddr
	apiPort             int
	host                string
	dockerHost          string
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
	RTMPAddr            optional.V[domain.NetAddr] // defaults to disabled, or 127.0.0.1:1935
	RTMPSAddr           optional.V[domain.NetAddr] // defaults to disabled, or 127.0.0.1:1936
	RTSPAddr            optional.V[domain.NetAddr] // defaults to disabled, or 127.0.0.1:8554
	RTSPSAddr           optional.V[domain.NetAddr] // defaults to disabled, or 127.0.0.1:8332
	APIPort             int                        // defaults to 9997
	Host                string                     // defaults to "localhost"
	DockerHost          string                     // defaults to "localhost" (used for connecting to Docker containers)
	StreamKey           StreamKey                  // defaults to "live"
	ImageName           string                     // defaults to "ghcr.io/rfwatson/mediamtx-alpine:latest"
	ChanSize            int                        // defaults to 64
	UpdateStateInterval time.Duration              // defaults to 5 seconds
	KeyPairs            domain.KeyPairs
	ContainerClient     ContainerClient
	InDocker            bool
	Logger              *slog.Logger
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

	dockerHost := defaultDockerHost
	if params.DockerHost != "" {
		uri, err := url.Parse(params.DockerHost)
		if err != nil {
			return nil, fmt.Errorf("parse Docker host URL: %w", err)
		}
		if uri.Hostname() == "" {
			return nil, errors.New("docker host URL must not be empty")
		}
		dockerHost = uri.Hostname()
	}

	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
	return &Actor{
		rtmpAddr:            toNetAddr(params.RTMPAddr, defaultRTMPPort),
		rtmpsAddr:           toNetAddr(params.RTMPSAddr, defaultRTMPSPort),
		rtspAddr:            toNetAddr(params.RTSPAddr, defaultRTSPPort),
		rtspsAddr:           toNetAddr(params.RTSPSAddr, defaultRTSPSPort),
		apiPort:             cmp.Or(params.APIPort, defaultAPIPort),
		host:                cmp.Or(params.Host, defaultHost),
		dockerHost:          dockerHost,
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

	// If we are not connected to the Octoplex network, publishing the port on
	// 0.0.0.0 is necessary to ensure it is possible to connect to the
	// mediaserver container from the orchestrator process.
	publishIP := "127.0.0.1"
	if !a.containerClient.HasDockerNetwork() {
		publishIP = "0.0.0.0"
	}
	portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:9997", publishIP, a.apiPort))

	// RTMP and RTSP ports bind to 127.0.0.1 by default.
	if !a.rtmpAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtmpAddr.IP, a.rtmpAddr.Port, 1935))
	}
	if !a.rtmpsAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtmpsAddr.IP, a.rtmpsAddr.Port, 1936))
	}
	if !a.rtspAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtspAddr.IP, a.rtspAddr.Port, 8554))
	}
	if !a.rtspsAddr.IsZero() {
		portSpecs = append(portSpecs, fmt.Sprintf("%s:%d:%d", a.rtspsAddr.IP, a.rtspsAddr.Port, 8332))
	}

	exposedPorts, portBindings, err := nat.ParsePortSpecs(portSpecs)
	if err != nil {
		return fmt.Errorf("parse port specs: %w", err)
	}

	// RTMP and RTSP URLs are passed in state mostly to ensure that late-joining clients
	// can receive them. It might be nice to clean this up, the URLs don't change
	// so aren't really stateful data and don't need to be sent in every update.
	a.state.RTMPURL = a.RTMPURL()
	a.state.RTMPSURL = a.RTMPSURL()
	a.state.RTSPURL = a.RTSPURL()
	a.state.RTSPSURL = a.RTSPSURL()

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
	if a.rtspAddr.IsZero() {
		args = append(args, "rtsp.enabled", false)
	} else {
		args = append(args, "rtsp.enabled", true, "rtsp.bind_addr", a.rtspAddr.IP, "rtsp.bind_port", a.rtspAddr.Port)
	}
	if a.rtspsAddr.IsZero() {
		args = append(args, "rtsps.enabled", false)
	} else {
		args = append(args, "rtsps.enabled", true, "rtsps.bind_addr", a.rtspsAddr.IP, "rtsps.bind_port", a.rtspsAddr.Port)
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
	var rtmpEncryptionString string
	if a.rtmpsAddr.IsZero() {
		rtmpEncryptionString = "no"
	} else {
		rtmpEncryptionString = "optional"
	}

	var certPath, keyPath string
	if a.keyPairs.Custom.IsZero() {
		certPath = tlsInternalCertPath
		keyPath = tlsInternalKeyPath
	} else {
		certPath = tlsCertPath
		keyPath = tlsKeyPath
	}

	cfg := Config{
		LogLevel:        "info",
		LogDestinations: []string{"stdout"},
		AuthMethod:      "internal",
		AuthInternalUsers: []User{
			{
				User:        "any",
				IPs:         []string{}, // allow any IP
				Permissions: []UserPermission{{Action: "publish"}},
			},
			{
				User:        "api",
				Pass:        a.pass,
				IPs:         []string{}, // allow any IP
				Permissions: []UserPermission{{Action: "read"}},
			},
			{
				User:        "api",
				Pass:        a.pass,
				IPs:         []string{}, // allow any IP
				Permissions: []UserPermission{{Action: "api"}},
			},
		},
		RTMP:           true,
		RTMPEncryption: rtmpEncryptionString,
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
	}

	rtspEnabled := !a.rtspAddr.IsZero()
	rtspsEnabled := !a.rtspsAddr.IsZero()

	applySharedRTSPConfig := func() {
		cfg.RTSP = true
		cfg.RTSPTransports = []string{"tcp"}

		switch {
		case rtspsEnabled && rtspEnabled:
			cfg.RTSPEncryption = "optional"
		case rtspsEnabled:
			cfg.RTSPEncryption = "strict"
		default:
			cfg.RTSPEncryption = "no"
		}
	}

	if !a.rtspAddr.IsZero() {
		applySharedRTSPConfig()

		cfg.RTSPAddress = ":8554"
	}

	if !a.rtspsAddr.IsZero() {
		applySharedRTSPConfig()

		cfg.RTSPSAddress = ":8332"
		cfg.RTSPServerCert = certPath
		cfg.RTSPServerKey = keyPath
	}

	return yaml.Marshal(cfg)
}

// C returns a channel that will receive the current state of the media server.
func (a *Actor) C() <-chan domain.Source {
	return a.stateC
}

// State returns the current state of the media server.
//
// Blocks if the actor is not started yet.
func (a *Actor) State() domain.Source {
	resultChan := make(chan domain.Source)
	a.actorC <- func() {
		resultChan <- *a.state
	}
	return <-resultChan
}

// Close closes the media server actor.
func (a *Actor) Close() error {
	if err := a.containerClient.RemoveContainers(
		context.Background(),
		a.containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: componentName}),
	); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	return nil
}

// actorLoop is the main loop of the media server actor. It exits when the
// actor is closed, or the parent context is cancelled.
func (a *Actor) actorLoop(ctx context.Context, containerStateC <-chan domain.Container, errC <-chan error) {
	updateStateT := time.NewTicker(a.updateStateInterval)
	updateStateT.Stop() // only start when the container starts
	defer updateStateT.Stop()

	sendState := func() { a.stateC <- *a.state }

	for {
		select {
		case containerState := <-containerStateC:
			if containerState.Status == domain.ContainerStatusRunning && a.state.Container.Status != domain.ContainerStatusRunning {
				// The container has moved into running state.
				// Start polling the API.
				updateStateT.Reset(a.updateStateInterval)
			}

			a.state.Container = containerState

			if a.state.Container.Status == domain.ContainerStatusExited {
				updateStateT.Stop()
				a.handleContainerExit(nil)
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
				a.logger.Error("Error from container client", "err", err, "id", shortID(a.state.Container.ID))
			}

			updateStateT.Stop()
			a.handleContainerExit(err)

			sendState()
		case <-updateStateT.C:
			path, err := fetchPath(a.pathURL(string(a.streamKey)), a.apiClient)
			if err != nil {
				a.logger.Error("Error fetching path", "err", err)
				continue
			}

			if path.Ready != a.state.Live {
				a.state.Live = path.Ready
				a.state.LiveChangedAt = time.Now()
				a.state.Tracks = path.Tracks
				sendState()
			}
		case action, ok := <-a.actorC:
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
func (a *Actor) handleContainerExit(err error) {
	if a.state.Container.ExitCode != nil {
		a.state.ExitReason = fmt.Sprintf("Server process exited with code %d.", *a.state.Container.ExitCode)
	} else {
		a.state.ExitReason = "Server process exited unexpectedly."
	}
	if err != nil {
		a.state.ExitReason += "\n\n" + err.Error()
	}

	a.state.Live = false
}

// RTMPURL returns the RTMP URL for the media server, accessible from the host.
func (a *Actor) RTMPURL() string {
	if a.rtmpAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtmp://%s:%d/%s", a.host, a.rtmpAddr.Port, a.streamKey)
}

// RTMPSURL returns the RTMPS URL for the media server, accessible from the host.
func (a *Actor) RTMPSURL() string {
	if a.rtmpsAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtmps://%s:%d/%s", a.host, a.rtmpsAddr.Port, a.streamKey)
}

// RTSPURL returns the RTSP URL for the media server, accessible from the host.
func (a *Actor) RTSPURL() string {
	if a.rtspAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtsp://%s:%d/%s", a.host, a.rtspAddr.Port, a.streamKey)
}

// RTSPSURL returns the RTSPS URL for the media server, accessible from the host.
func (a *Actor) RTSPSURL() string {
	if a.rtspsAddr.IsZero() {
		return ""
	}

	return fmt.Sprintf("rtsps://%s:%d/%s", a.host, a.rtspsAddr.Port, a.streamKey)
}

// RTMPInternalURL returns the RTMP URL for the media server, accessible from
// the app network.
func (a *Actor) RTMPInternalURL() string {
	// Container port, not host port:
	return fmt.Sprintf("rtmp://mediaserver:1935/%s?user=api&pass=%s", a.streamKey, a.pass)
}

// pathURL returns the URL for fetching a path.
//
// When running in Docker, the URL must only be accessible from the container,
// via the container hostname (i.e. mediaserver). In this case, it does not
// need to be exposed publicly.
//
// When running outside Docker, it's assumed that Octoplex is running on the
// Docker host, so the API should be accessible over localhost.
func (a *Actor) pathURL(path string) string {
	var hostname string
	if a.containerClient.HasDockerNetwork() {
		hostname = componentName
	} else {
		hostname = a.dockerHost
	}

	return fmt.Sprintf("https://api:%s@%s:%d/v3/paths/get/%s", a.pass, hostname, a.apiPort, path)
}

// healthCheckURL returns the URL for the health check, accessible from the
// container. It is logged to Docker's events log so must not include
// credentials.
func (a *Actor) healthCheckURL() string {
	return fmt.Sprintf("https://localhost:%d/v3/paths/list", a.apiPort)
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

// toNetAddr builds a domain.NetAddr from an optional NetAddr, with default
// values set to RTMP default bind config if needed. If the OptionalNetAddr is
// not enabled, a zero value is returned.
func toNetAddr(a optional.V[domain.NetAddr], defaultPort int) domain.NetAddr {
	if !a.Present {
		return domain.NetAddr{}
	}

	return domain.NetAddr{
		IP:   cmp.Or(a.Value.IP, defaultRTMPIP),
		Port: cmp.Or(a.Value.Port, defaultPort),
	}
}
