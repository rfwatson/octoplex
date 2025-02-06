package testhelpers

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

// MockDockerClient is a mock docker client.
//
// TODO: migrate to mockery.
type MockDockerClient struct {
	*client.Client

	ContainerStatsResponse io.ReadCloser
	EventsResponse         func() <-chan events.Message
	EventsErr              <-chan error
}

func (c *MockDockerClient) ContainerStats(context.Context, string, bool) (container.StatsResponseReader, error) {
	return container.StatsResponseReader{Body: c.ContainerStatsResponse}, nil
}

func (c *MockDockerClient) Events(context.Context, events.ListOptions) (<-chan events.Message, <-chan error) {
	return c.EventsResponse(), c.EventsErr
}
