package container

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/termstream/domain"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/google/uuid"
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
	ContainerList(context.Context, container.ListOptions) ([]types.Container, error)
	ContainerRemove(context.Context, string, container.RemoveOptions) error
	ContainerStart(context.Context, string, container.StartOptions) error
	ContainerStats(context.Context, string, bool) (container.StatsResponseReader, error)
	ContainerStop(context.Context, string, container.StopOptions) error
	ContainerWait(context.Context, string, container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	Events(context.Context, events.ListOptions) (<-chan events.Message, <-chan error)
	ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error)
	NetworkConnect(context.Context, string, string, *network.EndpointSettings) error
	NetworkCreate(context.Context, string, network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(context.Context, string) error
}

// Client provides a thin wrapper around the Docker API client, and provides
// additional functionality such as exposing container stats.
type Client struct {
	id        uuid.UUID
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	apiClient DockerClient
	networkID string
	logger    *slog.Logger
}

// NewClient creates a new Client.
func NewClient(ctx context.Context, apiClient DockerClient, logger *slog.Logger) (*Client, error) {
	id := uuid.New()
	network, err := apiClient.NetworkCreate(ctx, "termstream-"+id.String(), network.CreateOptions{Driver: "bridge"})
	if err != nil {
		return nil, fmt.Errorf("network create: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	client := &Client{
		id:        id,
		ctx:       ctx,
		cancel:    cancel,
		apiClient: apiClient,
		networkID: network.ID,
		logger:    logger,
	}

	return client, nil
}

// stats is a struct to hold container stats.
type stats struct {
	cpuPercent       float64
	memoryUsageBytes uint64
	rxRate, txRate   int
}

// getStats returns a channel that will receive container stats. The channel is
// never closed, but the spawned goroutine will exit when the context is
// cancelled.
func (a *Client) getStats(containerID string, networkCountConfig NetworkCountConfig) <-chan stats {
	ch := make(chan stats)

	go handleStats(a.ctx, containerID, a.apiClient, networkCountConfig, a.logger, ch)

	return ch
}

// getEvents returns a channel that will receive container events. The channel is
// never closed, but the spawned goroutine will exit when the context is
// cancelled.
func (a *Client) getEvents(containerID string) <-chan events.Message {
	sendC := make(chan events.Message)

	getEvents := func() (bool, error) {
		recvC, errC := a.apiClient.Events(a.ctx, events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("container", containerID),
				filters.Arg("type", "container"),
			),
		})

		for {
			select {
			case <-a.ctx.Done():
				return false, a.ctx.Err()
			case evt := <-recvC:
				sendC <- evt
			case err := <-errC:
				if a.ctx.Err() != nil || errors.Is(err, io.EOF) {
					return false, err
				}

				return true, err
			}
		}
	}

	go func() {
		for {
			shouldRetry, err := getEvents()
			if !shouldRetry {
				break
			}

			a.logger.Warn("Error receiving Docker events", "err", err, "id", shortID(containerID))
			time.Sleep(2 * time.Second)
		}
	}()

	return sendC
}

type NetworkCountConfig struct {
	Rx string // the network name to count the Rx bytes
	Tx string // the network name to count the Tx bytes
}

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name               string
	ChanSize           int
	ContainerConfig    *container.Config
	HostConfig         *container.HostConfig
	NetworkingConfig   *network.NetworkingConfig
	NetworkCountConfig NetworkCountConfig
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

		containerStateC <- domain.Container{State: "pulling"}

		pullReader, err := a.apiClient.ImagePull(ctx, params.ContainerConfig.Image, image.PullOptions{})
		if err != nil {
			sendError(fmt.Errorf("image pull: %w", err))
			return
		}
		_, _ = io.Copy(io.Discard, pullReader)
		_ = pullReader.Close()

		params.ContainerConfig.Labels["app"] = "termstream"
		params.ContainerConfig.Labels["app-id"] = a.id.String()

		var name string
		if params.Name != "" {
			name = "termstream-" + a.id.String() + "-" + params.Name
		}

		createResp, err := a.apiClient.ContainerCreate(
			ctx,
			params.ContainerConfig,
			params.HostConfig,
			params.NetworkingConfig,
			nil,
			name,
		)
		if err != nil {
			sendError(fmt.Errorf("container create: %w", err))
			return
		}
		containerStateC <- domain.Container{ID: createResp.ID, State: "created"}

		if err = a.apiClient.NetworkConnect(ctx, a.networkID, createResp.ID, nil); err != nil {
			sendError(fmt.Errorf("network connect: %w", err))
			return
		}

		if err = a.apiClient.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
			sendError(fmt.Errorf("container start: %w", err))
			return
		}
		a.logger.Info("Started container", "id", shortID(createResp.ID), "duration", time.Since(now))

		containerStateC <- domain.Container{ID: createResp.ID, State: "running"}

		a.runContainerLoop(ctx, createResp.ID, params.NetworkCountConfig, containerStateC, errC)
	}()

	return containerStateC, errC
}

// runContainerLoop is the control loop for a single container. It returns only
// when the container exits.
func (a *Client) runContainerLoop(
	ctx context.Context,
	containerID string,
	networkCountConfig NetworkCountConfig,
	stateC chan<- domain.Container,
	errC chan<- error,
) {
	statsC := a.getStats(containerID, networkCountConfig)
	eventsC := a.getEvents(containerID)
	containerRespC, containerErrC := a.apiClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	state := &domain.Container{ID: containerID, State: "running"}
	sendState := func() { stateC <- *state }
	sendState()

	for {
		select {
		case resp := <-containerRespC:
			a.logger.Info("Container entered non-running state", "exit_code", resp.StatusCode, "id", shortID(containerID))
			state.State = "exited"
			sendState()
			return
		case err := <-containerErrC:
			// TODO: error handling?
			if err != context.Canceled {
				a.logger.Error("Error setting container wait", "err", err, "id", shortID(containerID))
			}
			errC <- err
			return
		case evt := <-eventsC:
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
			}
		case stats := <-statsC:
			state.CPUPercent = stats.cpuPercent
			state.MemoryUsageBytes = stats.memoryUsageBytes
			state.RxRate = stats.rxRate
			state.TxRate = stats.txRate
			sendState()
		}
	}
}

// Close closes the client, stopping and removing all running containers.
func (a *Client) Close() error {
	a.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()

	containerList, err := a.containersMatchingLabels(ctx, nil)
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
func (a *Client) ContainerRunning(ctx context.Context, labels map[string]string) (bool, error) {
	containers, err := a.containersMatchingLabels(ctx, labels)
	if err != nil {
		return false, fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if container.State == "running" || container.State == "restarting" {
			return true, nil
		}
	}

	return false, nil
}

// RemoveContainers removes all containers with the given labels.
func (a *Client) RemoveContainers(ctx context.Context, labels map[string]string) error {
	containers, err := a.containersMatchingLabels(ctx, labels)
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

func (a *Client) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]types.Container, error) {
	filterArgs := filters.NewArgs(
		filters.Arg("label", "app=termstream"),
		filters.Arg("label", "app-id="+a.id.String()),
	)
	for k, v := range labels {
		filterArgs.Add("label", k+"="+v)
	}
	return a.apiClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
}

func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}
