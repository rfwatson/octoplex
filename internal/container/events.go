package container

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

func handleEvents(
	ctx context.Context,
	containerID string,
	apiClient DockerClient,
	logger *slog.Logger,
	ch chan events.Message,
) {
	getEvents := func() (bool, error) {
		recvC, errC := apiClient.Events(ctx, events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("container", containerID),
				filters.Arg("type", "container"),
			),
		})

		for {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case evt := <-recvC:
				ch <- evt
			case err := <-errC:
				if ctx.Err() != nil || errors.Is(err, io.EOF) {
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

			logger.Warn("Error receiving Docker events", "err", err, "id", shortID(containerID))
		}
	}()
}
