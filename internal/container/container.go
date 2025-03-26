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
	"github.com/docker/docker/errdefs"
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
	ContainerInspect(context.Context, string) (container.InspectResponse, error)
	ContainerList(context.Context, container.ListOptions) ([]container.Summary, error)
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
	networkID    string
	pulledImages map[string]struct{}
	logger       *slog.Logger
}

// NewClient creates a new Client.
func NewClient(ctx context.Context, apiClient DockerClient, logger *slog.Logger) (*Client, error) {
	id := shortid.New()
	network, err := apiClient.NetworkCreate(
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

	ctx, cancel := context.WithCancel(ctx)

	client := &Client{
		id:           id,
		ctx:          ctx,
		cancel:       cancel,
		apiClient:    apiClient,
		networkID:    network.ID,
		pulledImages: make(map[string]struct{}),
		logger:       logger,
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

// getStats returns a channel that will receive container stats. The channel is
// never closed, but any spawned goroutines will exit when the context is
// cancelled.
func (a *Client) getStats(containerID string, networkCountConfig NetworkCountConfig) <-chan stats {
	ch := make(chan stats)

	go handleStats(a.ctx, containerID, a.apiClient, networkCountConfig, a.logger, ch)

	return ch
}

// getEvents returns a channel that will receive container events. The channel is
// never closed, but any spawned goroutines will exit when the context is
// cancelled.
func (a *Client) getEvents(containerID string) <-chan events.Message {
	ch := make(chan events.Message)

	go handleEvents(a.ctx, containerID, a.apiClient, a.logger, ch)

	return ch
}

type NetworkCountConfig struct {
	Rx string // the network name to count the Rx bytes
	Tx string // the network name to count the Tx bytes
}

type CopyFileConfig struct {
	Path    string
	Payload io.Reader
	Mode    int64
}

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name               string
	ChanSize           int
	ContainerConfig    *container.Config
	HostConfig         *container.HostConfig
	NetworkingConfig   *network.NetworkingConfig
	NetworkCountConfig NetworkCountConfig
	CopyFileConfigs    []CopyFileConfig
}

// RunContainer runs a container with the given parameters.
//
// The returned state channel will receive the state of the container and will
// never be closed. The error channel will receive an error if the container
// fails to start, and will be closed when the container exits, possibly after
// receiving an error.
func (a *Client) RunContainer(ctx context.Context, params RunContainerParams) (<-chan domain.Container, <-chan error) {
	now := time.Now()
	containerStateC := make(chan domain.Container, cmp.Or(params.ChanSize, defaultChanSize))
	errC := make(chan error, 1)
	sendError := func(err error) {
		errC <- err
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer close(errC)

		if err := a.pullImageIfNeeded(ctx, params.ContainerConfig.Image, containerStateC); err != nil {
			a.logger.Error("Error pulling image", "err", err)
			sendError(fmt.Errorf("image pull: %w", err))
			return
		}

		containerConfig := *params.ContainerConfig
		containerConfig.Labels = make(map[string]string)
		maps.Copy(containerConfig.Labels, params.ContainerConfig.Labels)
		containerConfig.Labels[LabelApp] = domain.AppName
		containerConfig.Labels[LabelAppID] = a.id.String()

		var name string
		if params.Name != "" {
			name = domain.AppName + "-" + a.id.String() + "-" + params.Name
		}

		createResp, err := a.apiClient.ContainerCreate(
			ctx,
			&containerConfig,
			params.HostConfig,
			params.NetworkingConfig,
			nil,
			name,
		)
		if err != nil {
			sendError(fmt.Errorf("container create: %w", err))
			return
		}
		containerStateC <- domain.Container{ID: createResp.ID, Status: domain.ContainerStatusCreated}

		if err = a.apiClient.NetworkConnect(ctx, a.networkID, createResp.ID, nil); err != nil {
			sendError(fmt.Errorf("network connect: %w", err))
			return
		}

		if err = a.copyFilesToContainer(ctx, createResp.ID, params.CopyFileConfigs); err != nil {
			sendError(fmt.Errorf("copy files to container: %w", err))
			return
		}

		if err = a.apiClient.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
			sendError(fmt.Errorf("container start: %w", err))
			return
		}
		a.logger.Info("Started container", "id", shortID(createResp.ID), "duration", time.Since(now))

		containerStateC <- domain.Container{ID: createResp.ID, Status: domain.ContainerStatusRunning}

		a.runContainerLoop(
			ctx,
			createResp.ID,
			params.ContainerConfig.Image,
			params.NetworkCountConfig,
			containerStateC,
			errC,
		)
	}()

	return containerStateC, errC
}

func (a *Client) copyFilesToContainer(ctx context.Context, containerID string, fileConfigs []CopyFileConfig) error {
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

	if err := a.apiClient.CopyToContainer(ctx, containerID, "/", &buf, container.CopyToContainerOptions{}); err != nil {
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
func (a *Client) pullImageIfNeeded(ctx context.Context, imageName string, containerStateC chan<- domain.Container) error {
	a.mu.Lock()
	_, ok := a.pulledImages[imageName]
	a.mu.Unlock()

	if ok {
		return nil
	}

	if err := handleImagePull(ctx, imageName, a.apiClient, containerStateC); err != nil {
		return err
	}

	a.mu.Lock()
	a.pulledImages[imageName] = struct{}{}
	a.mu.Unlock()

	return nil
}

// runContainerLoop is the control loop for a single container. It returns only
// when the container exits.
func (a *Client) runContainerLoop(
	ctx context.Context,
	containerID string,
	imageName string,
	networkCountConfig NetworkCountConfig,
	stateC chan<- domain.Container,
	errC chan<- error,
) {
	type containerWaitResponse struct {
		container.WaitResponse
		restarting bool
	}

	containerRespC := make(chan containerWaitResponse)
	containerErrC := make(chan error)
	statsC := a.getStats(containerID, networkCountConfig)
	eventsC := a.getEvents(containerID)

	// ContainerWait only sends a result for the first non-running state, so we
	// need to poll it repeatedly.
	//
	// The goroutine exits when a value is received on the error channel, or when
	// the container exits and is not restarting, or when the context is cancelled.
	go func() {
		for {
			respC, errC := a.apiClient.ContainerWait(ctx, containerID, container.WaitConditionNextExit)
			select {
			case resp := <-respC:
				var restarting bool
				// Check if the container is restarting. If it is not then we don't
				// want to wait for it again and can return early.
				ctr, err := a.apiClient.ContainerInspect(ctx, containerID)
				// Race condition: the container may already have been removed.
				if errdefs.IsNotFound(err) {
					// ignore error but do not restart
				} else if err != nil {
					a.logger.Error("Error inspecting container", "err", err, "id", shortID(containerID))
					containerErrC <- err
					return
					// Race condition: the container may have already restarted.
				} else if ctr.State.Status == domain.ContainerStatusRestarting || ctr.State.Status == domain.ContainerStatusRunning {
					restarting = true
				}

				containerRespC <- containerWaitResponse{WaitResponse: resp, restarting: restarting}
				if !restarting {
					return
				}
			case err := <-errC:
				// Otherwise, this is probably unexpected and we need to handle it.
				containerErrC <- err
				return
			case <-ctx.Done():
				containerErrC <- ctx.Err()
				return
			}
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
			a.logger.Info("Container entered non-running state", "exit_code", resp.StatusCode, "id", shortID(containerID), "restarting", resp.restarting)

			var containerState string
			if resp.restarting {
				containerState = domain.ContainerStatusRestarting
			} else {
				containerState = domain.ContainerStatusExited
			}

			state.Status = containerState
			state.CPUPercent = 0
			state.MemoryUsageBytes = 0
			state.HealthState = "unhealthy"
			state.RxRate = 0
			state.TxRate = 0
			state.RxSince = time.Time{}
			state.RestartCount++

			if !resp.restarting {
				exitCode := int(resp.StatusCode)
				state.ExitCode = &exitCode
				sendState()
				return
			}

			sendState()
		case err := <-containerErrC:
			// TODO: error handling?
			if err != context.Canceled {
				a.logger.Error("Error setting container wait", "err", err, "id", shortID(containerID))
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
					a.logger.Warn("Unknown health status", "status", evt.Action)
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

// Close closes the client, stopping and removing all running containers.
func (a *Client) Close() error {
	a.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	containerList, err := a.containersMatchingLabels(ctx, a.instanceLabels())
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containerList {
		if err := a.removeContainer(ctx, container.ID); err != nil {
			a.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	a.wg.Wait()

	if a.networkID != "" {
		if err := a.apiClient.NetworkRemove(ctx, a.networkID); err != nil {
			a.logger.Error("Error removing network", "err", err)
		}
	}

	return a.apiClient.Close()
}

func (a *Client) removeContainer(ctx context.Context, id string) error {
	a.logger.Info("Stopping container", "id", shortID(id))
	stopTimeout := int(stopTimeout.Seconds())
	if err := a.apiClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("container stop: %w", err)
	}

	a.logger.Info("Removing container", "id", shortID(id))
	if err := a.apiClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}

	return nil
}

// ContainerRunning checks if a container with the given labels is running.
func (a *Client) ContainerRunning(ctx context.Context, labelOptions LabelOptions) (bool, error) {
	containers, err := a.containersMatchingLabels(ctx, labelOptions())
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
func (a *Client) RemoveContainers(ctx context.Context, labelOptions LabelOptions) error {
	containers, err := a.containersMatchingLabels(ctx, labelOptions())
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if err := a.removeContainer(ctx, container.ID); err != nil {
			a.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	return nil
}

// RemoveUnusedNetworks removes all networks that are not used by any
// container.
func (a *Client) RemoveUnusedNetworks(ctx context.Context) error {
	networks, err := a.otherNetworks(ctx)
	if err != nil {
		return fmt.Errorf("other networks: %w", err)
	}

	for _, network := range networks {
		a.logger.Info("Removing network", "id", shortID(network.ID))
		if err = a.apiClient.NetworkRemove(ctx, network.ID); err != nil {
			a.logger.Error("Error removing network", "err", err, "id", shortID(network.ID))
		}
	}

	return nil
}

func (a *Client) otherNetworks(ctx context.Context) ([]network.Summary, error) {
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", LabelApp+"="+domain.AppName)

	networks, err := a.apiClient.NetworkList(ctx, network.ListOptions{Filters: filterArgs})
	if err != nil {
		return nil, fmt.Errorf("network list: %w", err)
	}

	return slices.DeleteFunc(networks, func(n network.Summary) bool {
		return n.ID == a.networkID
	}), nil
}

// LabelOptions is a function that returns a map of labels.
type LabelOptions func() map[string]string

// ContainersWithLabels returns a LabelOptions function that returns the labels for
// this app instance.
func (a *Client) ContainersWithLabels(extraLabels map[string]string) LabelOptions {
	return func() map[string]string {
		return a.instanceLabels(extraLabels)
	}
}

// AllContainers returns a LabelOptions function that returns the labels for any
// app instance.
func AllContainers() LabelOptions {
	return func() map[string]string {
		return map[string]string{LabelApp: domain.AppName}
	}
}

func (a *Client) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]container.Summary, error) {
	filterArgs := filters.NewArgs()
	for k, v := range labels {
		filterArgs.Add("label", k+"="+v)
	}
	return a.apiClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
}

func (a *Client) instanceLabels(extraLabels ...map[string]string) map[string]string {
	labels := map[string]string{
		LabelApp:   domain.AppName,
		LabelAppID: a.id.String(),
	}

	for _, el := range extraLabels {
		for k, v := range el {
			labels[k] = v
		}
	}

	return labels
}

func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}
