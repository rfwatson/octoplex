package container

import (
	"archive/tar"
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/shortid"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// stopTimeout is the timeout for stopping a container.
	stopTimeout = 3 * time.Second

	// defaultChanSize is the default size of asynchronous non-error channels.
	defaultChanSize = 64
)

// DockerClient isolates a docker *client.Client.
type DockerClient interface {
	io.Closer

	ContainerCreate(context.Context, *container.Config, *container.HostConfig, *network.NetworkingConfig, *ocispec.Platform, string) (container.CreateResponse, error)
	ContainerList(context.Context, container.ListOptions) ([]container.Summary, error)
	ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error)
	ContainerRemove(context.Context, string, container.RemoveOptions) error
	ContainerStart(context.Context, string, container.StartOptions) error
	ContainerStats(context.Context, string, bool) (container.StatsResponseReader, error)
	ContainerStop(context.Context, string, container.StopOptions) error
	CopyToContainer(context.Context, string, string, io.Reader, container.CopyToContainerOptions) error
	ContainerWait(context.Context, string, container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	Events(context.Context, events.ListOptions) (<-chan events.Message, <-chan error)
	ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error)
	NetworkConnect(context.Context, string, string, *network.EndpointSettings) error
	NetworkCreate(context.Context, string, network.CreateOptions) (network.CreateResponse, error)
	NetworkDisconnect(context.Context, string, string, bool) error
	NetworkList(context.Context, network.ListOptions) ([]network.Summary, error)
	NetworkRemove(context.Context, string) error
}

const (
	LabelPrefix    = "io.netflux.octoplex."
	LabelApp       = LabelPrefix + "app"
	LabelAppID     = LabelPrefix + "app-id"
	LabelComponent = LabelPrefix + "component"
	LabelURL       = LabelPrefix + "url"
)

// Client provides a thin wrapper around the Docker API client, and provides
// additional functionality such as exposing container stats.
type Client struct {
	id           shortid.ID
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	wg           sync.WaitGroup
	apiClient    DockerClient
	inDocker     bool
	hasDockerNet bool // hasDockerNet is true if this client is running inside a Docker container and is connected to the Octoplex network.
	networkID    string
	cancelFuncs  map[string]context.CancelFunc
	pulledImages map[string]struct{}
	logger       *slog.Logger
}

// NewParams are the parameters for creating a new Client.
type NewParams struct {
	APIClient DockerClient
	InDocker  bool
	Logger    *slog.Logger
}

// NewClient creates a new Client.
func NewClient(ctx context.Context, params NewParams) (*Client, error) {
	id := shortid.New()
	network, err := params.APIClient.NetworkCreate(
		ctx,
		domain.AppName+"-"+id.String(),
		network.CreateOptions{
			Driver: "bridge",
			Labels: map[string]string{LabelApp: domain.AppName, LabelAppID: id.String()},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("network create: %w", err)
	}

	// connectDockerNetwork attempts to connect the current container (based on
	// the hostname) to the Octoplex-defined network. If we are not running as a
	// container this is a no-op. If connecting fails, it's not a fatal error. We
	// can try to handle networking differently, although there are scenarios
	// where it may fail.
	connectDockerNetwork := func() bool {
		if !params.InDocker {
			return false
		}

		containerID, err := selfContainerID()
		if err != nil {
			params.Logger.Info("Unable to get self container ID, not connecting to network", "err", err)
			return false
		}
		if err := params.APIClient.NetworkConnect(ctx, network.ID, containerID, nil); err != nil {
			params.Logger.Info("Unable to connect to Octoplex network", "err", err)
			return false
		}

		return true
	}

	ctx, cancel := context.WithCancel(ctx)
	client := &Client{
		id:           id,
		ctx:          ctx,
		cancel:       cancel,
		apiClient:    params.APIClient,
		inDocker:     params.InDocker,
		hasDockerNet: connectDockerNetwork(),
		networkID:    network.ID,
		cancelFuncs:  make(map[string]context.CancelFunc),
		pulledImages: make(map[string]struct{}),
		logger:       params.Logger,
	}

	return client, nil
}

// stats is a struct to hold container stats.
type stats struct {
	cpuPercent       float64
	memoryUsageBytes uint64
	rxRate, txRate   int
	rxSince          time.Time
}

// HasDockerNetwork returns true if the client is running inside a Docker
// container, and is connected to the Octoplex network.
func (c *Client) HasDockerNetwork() bool {
	return c.hasDockerNet
}

// getStats returns a channel that will receive container stats. The channel is
// never closed, but any spawned goroutines will exit when the context is
// cancelled.
func (c *Client) getStats(containerID string) <-chan stats {
	ch := make(chan stats)

	go handleStats(c.ctx, containerID, c.apiClient, c.logger, ch)

	return ch
}

// getEvents returns a channel that will receive container events. The channel is
// never closed, but any spawned goroutines will exit when the context is
// cancelled.
func (c *Client) getEvents(containerID string) <-chan events.Message {
	ch := make(chan events.Message)

	go handleEvents(c.ctx, containerID, c.apiClient, c.logger, ch)

	return ch
}

// getLogs returns a channel (which is never closed) that will receive
// container logs.
func (c *Client) getLogs(ctx context.Context, containerID string, cfg LogConfig) <-chan []byte {
	if !cfg.Stdout && !cfg.Stderr {
		return nil
	}

	ch := make(chan []byte)

	go getLogs(ctx, containerID, c.apiClient, cfg, ch, c.logger)

	return ch
}

// CopyFileConfig holds configuration for a single file which should be copied
// into a container.
type CopyFileConfig struct {
	Path    string
	Payload io.Reader
	Mode    int64
}

// LogConfig holds configuration for container logs.
type LogConfig struct {
	Stdout, Stderr bool
}

// ShouldRestartFunc is a callback function that is called when a container
// exits. It should return true if the container is to be restarted. If not
// restarting, err may be non-nil.
type ShouldRestartFunc func(
	exitCode int64,
	restartCount int,
	containerLogs [][]byte,
	runningTime time.Duration,
) (bool, error)

// defaultRestartInterval is the default interval between restarts.
// TODO: exponential backoff
const defaultRestartInterval = 10 * time.Second

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name             string
	ChanSize         int
	ContainerConfig  *container.Config
	HostConfig       *container.HostConfig
	NetworkingConfig *network.NetworkingConfig
	CopyFiles        []CopyFileConfig
	Logs             LogConfig
	ShouldRestart    ShouldRestartFunc
	RestartInterval  time.Duration // defaults to 10 seconds
}

// RunContainer runs a container with the given parameters.
//
// The returned state channel will receive the state of the container and will
// never be closed. The error channel will receive an error if the container
// fails to start, and will be closed when the container exits, possibly after
// receiving an error.
//
// Panics if ShouldRestart is non-nil and the host config defines a restart
// policy of its own.
func (c *Client) RunContainer(ctx context.Context, params RunContainerParams) (<-chan domain.Container, <-chan error) {
	if params.ShouldRestart != nil && !params.HostConfig.RestartPolicy.IsNone() {
		panic("shouldRestart and restart policy are mutually exclusive")
	}

	now := time.Now()
	containerStateC := make(chan domain.Container, cmp.Or(params.ChanSize, defaultChanSize))
	errC := make(chan error, 1)
	sendError := func(err error) { errC <- err }

	c.wg.Go(func() {
		defer close(errC)

		if err := c.pullImageIfNeeded(ctx, params.ContainerConfig.Image, containerStateC); err != nil {
			c.logger.Info("Error pulling image", "err", err)
		}

		containerConfig := *params.ContainerConfig
		containerConfig.Labels = make(map[string]string)
		maps.Copy(containerConfig.Labels, params.ContainerConfig.Labels)
		containerConfig.Labels[LabelApp] = domain.AppName
		containerConfig.Labels[LabelAppID] = c.id.String()

		hostConfig := *params.HostConfig
		hostConfig.NetworkMode = container.NetworkMode(c.networkID)

		var name string
		if params.Name != "" {
			name = domain.AppName + "-" + c.id.String() + "-" + params.Name
		}

		createResp, err := c.apiClient.ContainerCreate(
			ctx,
			&containerConfig,
			&hostConfig,
			params.NetworkingConfig,
			nil,
			name,
		)
		if err != nil {
			sendError(fmt.Errorf("container create: %w", err))
			return
		}
		containerStateC <- domain.Container{ID: createResp.ID, Status: domain.ContainerStatusCreated}

		if err = c.apiClient.NetworkConnect(ctx, c.networkID, createResp.ID, nil); err != nil {
			sendError(fmt.Errorf("network connect: %w", err))
			return
		}

		if err = c.copyFilesToContainer(ctx, createResp.ID, params.CopyFiles); err != nil {
			sendError(fmt.Errorf("copy files to container: %w", err))
			return
		}

		if err = c.apiClient.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
			sendError(fmt.Errorf("container start: %w", err))
			return
		}
		c.logger.Info("Started container", "id", shortID(createResp.ID), "duration", time.Since(now))

		containerStateC <- domain.Container{ID: createResp.ID, Status: domain.ContainerStatusRunning}

		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		c.mu.Lock()
		c.cancelFuncs[createResp.ID] = cancel
		c.mu.Unlock()

		c.runContainerLoop(
			ctx,
			cancel,
			createResp.ID,
			params.ContainerConfig.Image,
			params.Logs,
			params.ShouldRestart,
			cmp.Or(params.RestartInterval, defaultRestartInterval),
			containerStateC,
			errC,
		)
	})

	return containerStateC, errC
}

func (c *Client) copyFilesToContainer(ctx context.Context, containerID string, fileConfigs []CopyFileConfig) error {
	if len(fileConfigs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, fileConfig := range fileConfigs {
		payload, err := io.ReadAll(fileConfig.Payload)
		if err != nil {
			return fmt.Errorf("read payload: %w", err)
		}

		hdr := tar.Header{
			Name: fileConfig.Path,
			Mode: fileConfig.Mode,
			Size: int64(len(payload)),
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			return fmt.Errorf("write tar header: %w", err)
		}
		if _, err := tw.Write(payload); err != nil {
			return fmt.Errorf("write tar payload: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}

	if err := c.apiClient.CopyToContainer(ctx, containerID, "/", &buf, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copy to container: %w", err)
	}

	return nil
}

type pullProgressDetail struct {
	Curr  int64 `json:"current"`
	Total int64 `json:"total"`
}

type pullProgress struct {
	Status   string             `json:"status"`
	Detail   pullProgressDetail `json:"progressDetail"`
	Progress string             `json:"progress"`
}

// pullImageIfNeeded pulls the image if it has not already been pulled.
func (c *Client) pullImageIfNeeded(ctx context.Context, imageName string, containerStateC chan<- domain.Container) error {
	c.mu.Lock()
	_, ok := c.pulledImages[imageName]
	c.mu.Unlock()

	if ok {
		return nil
	}

	if err := handleImagePull(ctx, imageName, c.apiClient, containerStateC, c.logger); err != nil {
		return err
	}

	c.mu.Lock()
	c.pulledImages[imageName] = struct{}{}
	c.mu.Unlock()

	return nil
}

type containerWaitResponse struct {
	container.WaitResponse

	restarting   bool
	restartCount int
	err          error
}

// runContainerLoop is the control loop for a single container. It returns only
// when the container exits.
func (c *Client) runContainerLoop(
	ctx context.Context,
	cancel context.CancelFunc,
	containerID string,
	imageName string,
	logConfig LogConfig,
	shouldRestartFunc ShouldRestartFunc,
	restartInterval time.Duration,
	stateC chan<- domain.Container,
	errC chan<- error,
) {
	defer cancel()

	containerRespC := make(chan containerWaitResponse)
	containerErrC := make(chan error, 1)
	statsC := c.getStats(containerID)
	eventsC := c.getEvents(containerID)

	go func() {
		var restartCount int
		for c.waitForContainerExit(ctx, containerID, containerRespC, containerErrC, logConfig, shouldRestartFunc, restartInterval, restartCount) {
			restartCount++
		}
	}()

	state := &domain.Container{
		ID:        containerID,
		Status:    domain.ContainerStatusRunning,
		ImageName: imageName,
	}
	sendState := func() { stateC <- *state }
	sendState()

	for {
		select {
		case resp := <-containerRespC:
			c.logger.Info("Container entered non-running state", "exit_code", resp.StatusCode, "id", shortID(containerID), "restarting", resp.restarting)

			var containerState string
			var containerErr error
			if resp.restarting {
				containerState = domain.ContainerStatusRestarting
			} else {
				containerState = domain.ContainerStatusExited
				containerErr = resp.err
			}

			state.Status = containerState
			state.Err = containerErr
			state.RestartCount = resp.restartCount
			state.CPUPercent = 0
			state.MemoryUsageBytes = 0
			state.HealthState = "unhealthy"
			state.RxRate = 0
			state.TxRate = 0
			state.RxSince = time.Time{}

			if !resp.restarting {
				exitCode := int(resp.StatusCode)
				state.ExitCode = &exitCode
				sendState()
				return
			}

			sendState()
		case err := <-containerErrC:
			// TODO: verify error handling
			if err != context.Canceled {
				c.logger.Error("Error setting container wait", "err", err, "id", shortID(containerID))
			}
			errC <- err
			return
		case evt := <-eventsC:
			if evt.Type != events.ContainerEventType {
				continue
			}

			if evt.Action == "start" {
				state.Status = domain.ContainerStatusRunning
				sendState()

				continue
			}

			if strings.Contains(string(evt.Action), "health_status") {
				switch evt.Action {
				case events.ActionHealthStatusRunning:
					state.HealthState = "running"
				case events.ActionHealthStatusHealthy:
					state.HealthState = "healthy"
				case events.ActionHealthStatusUnhealthy:
					state.HealthState = "unhealthy"
				default:
					c.logger.Warn("Unknown health status", "status", evt.Action)
					state.HealthState = "unknown"
				}
				sendState()
				continue
			}
		case s := <-statsC:
			state.CPUPercent = s.cpuPercent
			state.MemoryUsageBytes = s.memoryUsageBytes
			state.RxRate = s.rxRate
			state.TxRate = s.txRate
			state.RxSince = s.rxSince
			sendState()
		}
	}
}

// waitForContainerExit blocks while it waits for a container to exit, and restarts
// it if configured to do so.
func (c *Client) waitForContainerExit(
	ctx context.Context,
	containerID string,
	containerRespC chan<- containerWaitResponse,
	containerErrC chan<- error,
	logConfig LogConfig,
	shouldRestartFunc ShouldRestartFunc,
	restartInterval time.Duration,
	restartCount int,
) bool {
	var logs [][]byte
	startedWaitingAt := time.Now()
	respC, errC := c.apiClient.ContainerWait(ctx, containerID, container.WaitConditionNextExit)
	logsC := c.getLogs(ctx, containerID, logConfig)

	timer := time.NewTimer(restartInterval)
	defer timer.Stop()
	timer.Stop()

	for {
		select {
		case resp := <-respC:
			exit := func(err error) {
				c.logger.Info("Container exited", "id", shortID(containerID), "should_restart", "false", "exit_code", resp.StatusCode, "restart_count", restartCount)
				containerRespC <- containerWaitResponse{
					WaitResponse: resp,
					restarting:   false,
					restartCount: restartCount,
					err:          err,
				}
			}

			// If the container exited with a non-zero status code, and debug
			// logging is not enabled, log the container logs at ERROR level for
			// debugging.
			// TODO: parameterize
			if resp.StatusCode != 0 && !c.logger.Enabled(ctx, slog.LevelDebug) {
				for _, line := range logs {
					c.logger.Error("Container log", "id", shortID(containerID), "log", string(line))
				}
			}

			if shouldRestartFunc == nil {
				exit(nil)
				return false
			}

			shouldRestart, err := shouldRestartFunc(resp.StatusCode, restartCount, logs, time.Since(startedWaitingAt))
			if shouldRestart && err != nil {
				panic(fmt.Errorf("shouldRestart must return nil error if restarting, but returned: %w", err))
			}
			if !shouldRestart {
				exit(err)
				return false
			}

			c.logger.Info("Container exited", "id", shortID(containerID), "should_restart", "true", "exit_code", resp.StatusCode, "restart_count", restartCount)
			timer.Reset(restartInterval)

			containerRespC <- containerWaitResponse{
				WaitResponse: resp,
				restarting:   true,
				restartCount: restartCount,
			}
			// Don't return yet. Wait for the timer to fire.
		case <-timer.C:
			c.logger.Info("Container restarting", "id", shortID(containerID), "restart_count", restartCount)
			if err := c.apiClient.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
				containerErrC <- fmt.Errorf("container start: %w", err)
				return false
			}
			c.logger.Info("Restarted container", "id", shortID(containerID))
			return true
		case line := <-logsC:
			c.logger.Debug("Container log", "id", shortID(containerID), "log", string(line))
			// TODO: limit max stored lines
			logs = append(logs, line)
		case err := <-errC:
			containerErrC <- err
			return false
		case <-ctx.Done():
			// This is probably because the container was stopped.
			containerRespC <- containerWaitResponse{WaitResponse: container.WaitResponse{}, restarting: false}
			return false
		}
	}
}

// Close closes the client, stopping and removing all running containers.
func (c *Client) Close() error {
	c.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	containerList, err := c.containersMatchingLabels(ctx, c.instanceLabels())
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containerList {
		if err := c.removeContainer(ctx, container.ID); err != nil {
			c.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	c.wg.Wait()

	if c.networkID != "" {
		if c.inDocker {
			containerID, err := selfContainerID()
			if err != nil {
				c.logger.Error("Error getting self ID", "err", err)
			} else {
				if err := c.apiClient.NetworkDisconnect(ctx, c.networkID, containerID, true); err != nil {
					c.logger.Error("Error disconnecting from network", "err", err)
				}
			}
		}

		if err := c.apiClient.NetworkRemove(ctx, c.networkID); err != nil {
			c.logger.Error("Error removing network", "err", err)
		}
	}

	return c.apiClient.Close()
}

func (c *Client) removeContainer(ctx context.Context, id string) error {
	c.mu.Lock()
	cancel, ok := c.cancelFuncs[id]
	if ok {
		delete(c.cancelFuncs, id)
	}
	c.mu.Unlock()

	if ok {
		cancel()
	} else {
		// It is attempted to keep track of cancel functions for each container,
		// which allow clean cancellation of container restart logic during
		// removal. But there are legitimate occasions where the cancel function
		// would not exist (e.g. during startup check) and in general the state of
		// the Docker engine is preferred to local state in this package.
		c.logger.Debug("removeContainer: cancelFunc not found", "id", shortID(id))
	}

	c.logger.Info("Stopping container", "id", shortID(id))
	stopTimeout := int(stopTimeout.Seconds())
	if err := c.apiClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("container stop: %w", err)
	}

	c.logger.Info("Removing container", "id", shortID(id))
	if err := c.apiClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}

	return nil
}

// ContainerRunning checks if a container with the given labels is running.
func (c *Client) ContainerRunning(ctx context.Context, labelOptions LabelOptions) (bool, error) {
	containers, err := c.containersMatchingLabels(ctx, labelOptions())
	if err != nil {
		return false, fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if container.State == domain.ContainerStatusRunning || container.State == domain.ContainerStatusRestarting {
			return true, nil
		}
	}

	return false, nil
}

// RemoveContainers removes all containers with the given labels.
func (c *Client) RemoveContainers(ctx context.Context, labelOptions LabelOptions) error {
	containers, err := c.containersMatchingLabels(ctx, labelOptions())
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if err := c.removeContainer(ctx, container.ID); err != nil {
			c.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	return nil
}

// RemoveUnusedNetworks removes all networks that are not used by any
// container.
func (c *Client) RemoveUnusedNetworks(ctx context.Context) error {
	networks, err := c.otherNetworks(ctx)
	if err != nil {
		return fmt.Errorf("other networks: %w", err)
	}

	for _, network := range networks {
		c.logger.Info("Removing network", "id", shortID(network.ID))
		if err = c.apiClient.NetworkRemove(ctx, network.ID); err != nil {
			c.logger.Error("Error removing network", "err", err, "id", shortID(network.ID))
		}
	}

	return nil
}

func (c *Client) otherNetworks(ctx context.Context) ([]network.Summary, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", LabelApp+"="+domain.AppName)

	networks, err := c.apiClient.NetworkList(ctx, network.ListOptions{Filters: filterArgs})
	if err != nil {
		return nil, fmt.Errorf("network list: %w", err)
	}

	return slices.DeleteFunc(networks, func(n network.Summary) bool {
		return n.ID == c.networkID
	}), nil
}

// LabelOptions is a function that returns a map of labels.
type LabelOptions func() map[string]string

// ContainersWithLabels returns a LabelOptions function that returns the labels for
// this app instance.
func (c *Client) ContainersWithLabels(extraLabels map[string]string) LabelOptions {
	return func() map[string]string {
		return c.instanceLabels(extraLabels)
	}
}

// AllContainers returns a LabelOptions function that returns the labels for any
// app instance.
func AllContainers() LabelOptions {
	return func() map[string]string {
		return map[string]string{LabelApp: domain.AppName}
	}
}

func (c *Client) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]container.Summary, error) {
	filterArgs := filters.NewArgs()
	for k, v := range labels {
		filterArgs.Add("label", k+"="+v)
	}
	return c.apiClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
}

func (c *Client) instanceLabels(extraLabels ...map[string]string) map[string]string {
	labels := map[string]string{
		LabelApp:   domain.AppName,
		LabelAppID: c.id.String(),
	}

	for _, el := range extraLabels {
		for k, v := range el {
			labels[k] = v
		}
	}

	return labels
}

// selfContainerID returns the ID of the current container, inferred from the
// result of [os.Hostname].
//
// TODO: make this more robust.
func selfContainerID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("get hostname: %w", err)
	}

	return hostname, nil
}

func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}
