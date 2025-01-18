package container

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

var containerStopTimeout = 10 * time.Second

// Runner is responsible for running containers.
type Runner struct {
	id        uuid.UUID
	wg        sync.WaitGroup // TODO: is it needed?
	apiClient *client.Client
	logger    *slog.Logger
}

// NewRunner creates a new Runner.
func NewRunner(logger *slog.Logger) (*Runner, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &Runner{
		id:        uuid.New(),
		apiClient: apiClient,
		logger:    logger,
	}, nil
}

// Close closes the runner, stopping and removing all running containers.
func (r *Runner) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), containerStopTimeout)
	defer cancel()

	containerList, err := r.containersMatchingLabels(ctx, nil)
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containerList {
		if err := r.removeContainer(ctx, container.ID); err != nil {
			r.logger.Error("Error removing container:", "err", err)
		}
	}

	r.wg.Wait()

	return r.apiClient.Close()
}

func (r *Runner) removeContainer(ctx context.Context, id string) error {
	r.logger.Info("Stopping container")
	stopTimeout := int(containerStopTimeout.Seconds())
	if err := r.apiClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("container stop: %w", err)
	}

	r.logger.Info("Removing container")
	if err := r.apiClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}

	return nil
}

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name        string
	Image       string
	Env         []string
	Labels      map[string]string
	NetworkMode string
}

// RunContainer runs a container with the given parameters.
func (r *Runner) RunContainer(ctx context.Context, params RunContainerParams) (<-chan struct{}, error) {
	pullReader, err := r.apiClient.ImagePull(ctx, params.Image, image.PullOptions{})
	if err != nil {
		return nil, fmt.Errorf("image pull: %w", err)
	}
	_, _ = io.Copy(io.Discard, pullReader)
	_ = pullReader.Close()

	labels := map[string]string{
		"app":    "termstream",
		"app-id": r.id.String(),
	}
	maps.Copy(labels, params.Labels)

	var name string
	if params.Name != "" {
		name = "termstream-" + r.id.String() + "-" + params.Name
	}

	ctr, err := r.apiClient.ContainerCreate(
		ctx,
		&container.Config{
			Image:  params.Image,
			Env:    params.Env,
			Labels: labels,
		},
		&container.HostConfig{
			NetworkMode: container.NetworkMode(cmp.Or(params.NetworkMode, "default")),
		},
		nil,
		nil,
		name,
	)
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}

	if err = r.apiClient.ContainerStart(ctx, ctr.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("container start: %w", err)
	}
	r.logger.Info("Started container", "id", ctr.ID)

	ch := make(chan struct{}, 1)
	r.wg.Add(1)

	go func() {
		defer r.wg.Done()

		respChan, errChan := r.apiClient.ContainerWait(ctx, ctr.ID, container.WaitConditionNotRunning)
		select {
		case resp := <-respChan:
			r.logger.Info("Container terminated", "status", resp.StatusCode)
		case err = <-errChan:
			if err != context.Canceled {
				r.logger.Error("Container terminated with error", "err", err)
			}
		}

		ch <- struct{}{}
	}()

	return ch, nil
}

// ContainerRunning checks if a container with the given labels is running.
func (r *Runner) ContainerRunning(ctx context.Context, labels map[string]string) (bool, error) {
	containers, err := r.containersMatchingLabels(ctx, labels)
	if err != nil {
		return false, fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if container.State == "running" {
			return true, nil
		}
	}

	return false, nil
}

// RemoveContainers removes all containers with the given labels.
func (r *Runner) RemoveContainers(ctx context.Context, labels map[string]string) error {
	containers, err := r.containersMatchingLabels(ctx, labels)
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if err := r.removeContainer(ctx, container.ID); err != nil {
			r.logger.Error("Error removing container:", "err", err)
		}
	}

	return nil
}

func (r *Runner) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]types.Container, error) {
	filterArgs := filters.NewArgs(
		filters.Arg("label", "app=termstream"),
		filters.Arg("label", "app-id="+r.id.String()),
	)
	for k, v := range labels {
		filterArgs.Add("label", k+"="+v)
	}
	return r.apiClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
}
