package container

import (
	"context"
	"encoding/json"
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
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

// stopTimeout is the timeout for stopping a container.
var stopTimeout = 10 * time.Second

// Client provides a thin wrapper around the Docker API client, and provides
// additional functionality such as exposing container stats.
type Client struct {
	id        uuid.UUID
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	apiClient *client.Client
	logger    *slog.Logger
}

// NewClient creates a new Client.
func NewClient(ctx context.Context, logger *slog.Logger) (*Client, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	client := &Client{
		id:        uuid.New(),
		ctx:       ctx,
		cancel:    cancel,
		apiClient: apiClient,
		logger:    logger,
	}

	return client, nil
}

// stats is a struct to hold container stats.
type stats struct {
	cpuPercent       float64
	memoryUsageBytes uint64
}

// getStats returns a channel that will receive container stats. The channel is
// never closed, but the spawned goroutine will exit when the context is
// cancelled.
func (a *Client) getStats(containerID string) <-chan stats {
	ch := make(chan stats)

	go func() {
		statsReader, err := a.apiClient.ContainerStats(a.ctx, containerID, true)
		if err != nil {
			// TODO: error handling?
			a.logger.Error("Error getting container stats", "err", err, "id", shortID(containerID))
			return
		}
		defer statsReader.Body.Close()

		buf := make([]byte, 4_096)
		for {
			n, err := statsReader.Body.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					break
				}
				a.logger.Error("Error reading stats", "err", err, "id", shortID(containerID))
				break
			}

			var statsResp container.StatsResponse
			if err = json.Unmarshal(buf[:n], &statsResp); err != nil {
				a.logger.Error("Error unmarshalling stats", "err", err, "id", shortID(containerID))
				break
			}

			// https://stackoverflow.com/a/30292327/62871
			cpuDelta := float64(statsResp.CPUStats.CPUUsage.TotalUsage - statsResp.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(statsResp.CPUStats.SystemUsage - statsResp.PreCPUStats.SystemUsage)
			ch <- stats{
				cpuPercent:       (cpuDelta / systemDelta) * float64(statsResp.CPUStats.OnlineCPUs) * 100,
				memoryUsageBytes: statsResp.MemoryStats.Usage,
			}
		}
	}()

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

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name            string
	ContainerConfig *container.Config
	HostConfig      *container.HostConfig
}

// RunContainer runs a container with the given parameters.
//
// The returned channel will receive the current state of the container, and
// will be closed after the container has stopped.
func (a *Client) RunContainer(ctx context.Context, params RunContainerParams) (string, <-chan domain.ContainerState, error) {
	pullReader, err := a.apiClient.ImagePull(ctx, params.ContainerConfig.Image, image.PullOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("image pull: %w", err)
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
		nil,
		nil,
		name,
	)
	if err != nil {
		return "", nil, fmt.Errorf("container create: %w", err)
	}

	if err = a.apiClient.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
		return "", nil, fmt.Errorf("container start: %w", err)
	}
	a.logger.Info("Started container", "id", shortID(createResp.ID))

	containerStateC := make(chan domain.ContainerState, 1)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer close(containerStateC)

		a.runContainerLoop(ctx, createResp.ID, containerStateC)
	}()

	return createResp.ID, containerStateC, nil
}

// runContainerLoop is the control loop for a single container. It returns only
// when the container exits.
func (a *Client) runContainerLoop(ctx context.Context, containerID string, stateCh chan<- domain.ContainerState) {
	statsC := a.getStats(containerID)
	eventsC := a.getEvents(containerID)
	respC, errC := a.apiClient.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	state := &domain.ContainerState{ID: containerID}
	sendState := func() { stateCh <- *state }
	sendState()

	for {
		select {
		case resp := <-respC:
			a.logger.Info("Container entered non-running state", "exit_code", resp.StatusCode, "id", shortID(containerID))
			return
		case err := <-errC:
			// TODO: error handling?
			if err != context.Canceled {
				a.logger.Error("Error setting container wait", "err", err, "id", shortID(containerID))
			}
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
