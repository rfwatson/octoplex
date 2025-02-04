package testhelpers

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// MockDockerClient is a mock docker client.
type MockDockerClient struct {
	*client.Client

	ContainerStatsResponse io.ReadCloser
}

func (c *MockDockerClient) ContainerStats(context.Context, string, bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{Body: c.ContainerStatsResponse}, nil
}
