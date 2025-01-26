package container_test

import (
	"context"
	"testing"
	"time"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/testhelpers"
	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)
	defer apiClient.Close()

	logger := testhelpers.NewTestLogger()
	containerName := "termstream-test-" + uuid.NewString()
	component := "test-start-stop"

	client, err := container.NewClient(logger)
	require.NoError(t, err)

	running, err := client.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	containerID, _, err := client.RunContainer(ctx, container.RunContainerParams{
		Name: containerName,
		ContainerConfig: &typescontainer.Config{
			Image:  "bluenviron/mediamtx",
			Labels: map[string]string{"component": component},
		},
		HostConfig: &typescontainer.HostConfig{
			NetworkMode: "default",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, containerID)

	require.Eventually(
		t,
		func() bool {
			running, err = client.ContainerRunning(ctx, map[string]string{"component": component})
			return err == nil && running
		},
		2*time.Second,
		100*time.Millisecond,
		"container not in RUNNING state",
	)

	client.Close()

	running, err = client.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)
}
