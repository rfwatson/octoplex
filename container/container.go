package container

import (
	"cmp"
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

var (
	stopTimeout     = 10 * time.Second
	defaultChanSize = 64
)

type action func()

// Actor is an actor that provides a thin wrapper around the Docker API client,
// and provides additional functionality such as exposing container stats.
type Actor struct {
	id        uuid.UUID
	ctx       context.Context
	cancel    context.CancelFunc
	ch        chan action
	wg        sync.WaitGroup
	apiClient *client.Client
	logger    *slog.Logger

	// mutable state
	containers map[string]*domain.Container
}

// NewActorParams are the parameters for creating a new Actor.
type NewActorParams struct {
	ChanSize int
	Logger   *slog.Logger
}

// NewActor creates a new Actor.
func NewActor(ctx context.Context, params NewActorParams) (*Actor, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	client := &Actor{
		id:         uuid.New(),
		ctx:        ctx,
		cancel:     cancel,
		ch:         make(chan action, cmp.Or(params.ChanSize, defaultChanSize)),
		apiClient:  apiClient,
		logger:     params.Logger,
		containers: make(map[string]*domain.Container),
	}

	client.wg.Add(1)
	go func() {
		defer client.wg.Done()

		client.dockerEventLoop()
	}()

	go client.actorLoop()

	return client, nil
}

// actorLoop is the main loop for the client.
//
// It continues to run until the internal channel is closed, which only happens
// when [Close] is called. This means that it is not reliant on any context
// remaining open, and any method calls made after [Close] may deadlock or
// fail.
func (a *Actor) actorLoop() {
	for action := range a.ch {
		action()
	}
}

func (a *Actor) handleStats(id string) {
	statsReader, err := a.apiClient.ContainerStats(a.ctx, id, true)
	if err != nil {
		// TODO: error handling?
		a.logger.Error("Error getting container stats", "err", err, "id", shortID(id))
		return
	}
	defer statsReader.Body.Close()

	buf := make([]byte, 4_096)
	for {
		n, err := statsReader.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return
			}
			a.logger.Error("Error reading stats", "err", err, "id", shortID(id))
			return
		}

		var statsResp container.StatsResponse
		if err = json.Unmarshal(buf[:n], &statsResp); err != nil {
			a.logger.Error("Error unmarshalling stats", "err", err, "id", shortID(id))
			continue
		}

		a.ch <- func() {
			ctr, ok := a.containers[id]
			if !ok {
				return
			}

			// https://stackoverflow.com/a/30292327/62871
			cpuDelta := float64(statsResp.CPUStats.CPUUsage.TotalUsage - statsResp.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(statsResp.CPUStats.SystemUsage - statsResp.PreCPUStats.SystemUsage)
			ctr.CPUPercent = (cpuDelta / systemDelta) * float64(statsResp.CPUStats.OnlineCPUs) * 100
			ctr.MemoryUsageBytes = statsResp.MemoryStats.Usage
		}
	}
}

func (a *Actor) dockerEventLoop() {
	for {
		ch, errCh := a.apiClient.Events(a.ctx, events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("label", "app=termstream"),
				filters.Arg("label", "app-id="+a.id.String()),
			),
		})

		select {
		case <-a.ctx.Done():
			return
		case evt := <-ch:
			a.handleDockerEvent(evt)
		case err := <-errCh:
			if a.ctx.Err() != nil {
				return
			}

			a.logger.Warn("Error receiving Docker events", "err", err)
			time.Sleep(1 * time.Second)
		}
	}
}

func (a *Actor) handleDockerEvent(evt events.Message) {
	a.ch <- func() {
		ctr, ok := a.containers[evt.ID]
		if !ok {
			return
		}

		if strings.HasPrefix(string(evt.Action), "health_status") {
			a.logger.Info("Event: health status changed", "type", evt.Type, "action", evt.Action, "id", shortID(evt.ID))

			switch evt.Action {
			case events.ActionHealthStatusRunning:
				ctr.HealthState = "running"
			case events.ActionHealthStatusHealthy:
				ctr.HealthState = "healthy"
			case events.ActionHealthStatusUnhealthy:
				ctr.HealthState = "unhealthy"
			default:
				a.logger.Warn("Unknown health status", "action", evt.Action)
				ctr.HealthState = "unknown"
			}
		}
	}
}

// GetContainerState returns a copy of the current state of all containers.
func (a *Actor) GetContainerState() map[string]domain.Container {
	resultChan := make(chan map[string]domain.Container)

	a.ch <- func() {
		result := make(map[string]domain.Container, len(a.containers))
		for id, ctr := range a.containers {
			result[id] = *ctr
		}
		resultChan <- result
	}

	return <-resultChan
}

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name            string
	ContainerConfig *container.Config
	HostConfig      *container.HostConfig
}

// RunContainer runs a container with the given parameters.
func (a *Actor) RunContainer(ctx context.Context, params RunContainerParams) (string, <-chan struct{}, error) {
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

	go a.handleStats(createResp.ID)

	ch := make(chan struct{}, 1)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		respChan, errChan := a.apiClient.ContainerWait(ctx, createResp.ID, container.WaitConditionNotRunning)
		select {
		case resp := <-respChan:
			a.logger.Info("Container entered non-running state", "exit_code", resp.StatusCode, "id", shortID(createResp.ID))
			a.ch <- func() {
				delete(a.containers, createResp.ID)
			}
		case err = <-errChan:
			// TODO: error handling?
			if err != context.Canceled {
				a.logger.Error("Error setting container wait", "err", err, "id", shortID(createResp.ID))
			}
		}

		ch <- struct{}{}
	}()

	// Update the containers map which must be done on the actor loop, but before
	// we return from the method.
	done := make(chan struct{})
	a.ch <- func() {
		a.containers[createResp.ID] = &domain.Container{ID: createResp.ID, HealthState: "healthy"}
		done <- struct{}{}
	}
	<-done

	return createResp.ID, ch, nil
}

// Close closes the client, stopping and removing all running containers.
func (a *Actor) Close() error {
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

	close(a.ch)

	return a.apiClient.Close()
}

func (a *Actor) removeContainer(ctx context.Context, id string) error {
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
func (a *Actor) ContainerRunning(ctx context.Context, labels map[string]string) (bool, error) {
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
func (a *Actor) RemoveContainers(ctx context.Context, labels map[string]string) error {
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

func (a *Actor) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]types.Container, error) {
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
