package container

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

// Client is a thin wrapper around the Docker client.
type Client struct {
	id        uuid.UUID
	wg        sync.WaitGroup // TODO: is it needed?
	apiClient *client.Client
	logger    *slog.Logger
}

// NewClient creates a new Client.
func NewClient(logger *slog.Logger) (*Client, error) {
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	return &Client{
		id:        uuid.New(),
		apiClient: apiClient,
		logger:    logger,
	}, nil
}

// RunContainerParams are the parameters for running a container.
type RunContainerParams struct {
	Name            string
	ContainerConfig *container.Config
	HostConfig      *container.HostConfig
}

// RunContainer runs a container with the given parameters.
func (r *Client) RunContainer(ctx context.Context, params RunContainerParams) (string, <-chan struct{}, error) {
	pullReader, err := r.apiClient.ImagePull(ctx, params.ContainerConfig.Image, image.PullOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("image pull: %w", err)
	}
	_, _ = io.Copy(io.Discard, pullReader)
	_ = pullReader.Close()

	params.ContainerConfig.Labels["app"] = "termstream"
	params.ContainerConfig.Labels["app-id"] = r.id.String()

	var name string
	if params.Name != "" {
		name = "termstream-" + r.id.String() + "-" + params.Name
	}

	createResp, err := r.apiClient.ContainerCreate(
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

	ch := make(chan struct{}, 1)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		respChan, errChan := r.apiClient.ContainerWait(ctx, createResp.ID, container.WaitConditionNotRunning)
		select {
		case resp := <-respChan:
			r.logger.Info("Container entered non-running state", "status", resp.StatusCode, "id", shortID(createResp.ID))
		case err = <-errChan:
			if err != context.Canceled {
				r.logger.Error("Error setting container wait", "err", err, "id", shortID(createResp.ID))
			}
		}

		ch <- struct{}{}
	}()

	if err = r.apiClient.ContainerStart(ctx, createResp.ID, container.StartOptions{}); err != nil {
		return "", nil, fmt.Errorf("container start: %w", err)
	}
	r.logger.Info("Started container", "id", shortID(createResp.ID))

	ctr, err := r.apiClient.ContainerInspect(ctx, createResp.ID)
	if err != nil {
		return "", nil, fmt.Errorf("container inspect: %w", err)
	}

	return ctr.ID, ch, nil
}

// Close closes the client, stopping and removing all running containers.
func (r *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), containerStopTimeout)
	defer cancel()

	containerList, err := r.containersMatchingLabels(ctx, nil)
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containerList {
		if err := r.removeContainer(ctx, container.ID); err != nil {
			r.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	r.wg.Wait()

	return r.apiClient.Close()
}

func (r *Client) removeContainer(ctx context.Context, id string) error {
	r.logger.Info("Stopping container", "id", shortID(id))
	stopTimeout := int(containerStopTimeout.Seconds())
	if err := r.apiClient.ContainerStop(ctx, id, container.StopOptions{Timeout: &stopTimeout}); err != nil {
		return fmt.Errorf("container stop: %w", err)
	}

	r.logger.Info("Removing container", "id", shortID(id))
	if err := r.apiClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("container remove: %w", err)
	}

	return nil
}

// ContainerRunning checks if a container with the given labels is running.
func (r *Client) ContainerRunning(ctx context.Context, labels map[string]string) (bool, error) {
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
func (r *Client) RemoveContainers(ctx context.Context, labels map[string]string) error {
	containers, err := r.containersMatchingLabels(ctx, labels)
	if err != nil {
		return fmt.Errorf("container list: %w", err)
	}

	for _, container := range containers {
		if err := r.removeContainer(ctx, container.ID); err != nil {
			r.logger.Error("Error removing container:", "err", err, "id", shortID(container.ID))
		}
	}

	return nil
}

func (r *Client) containersMatchingLabels(ctx context.Context, labels map[string]string) ([]types.Container, error) {
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

func shortID(id string) string {
	if len(id) < 12 {
		return id
	}
	return id[:12]
}
