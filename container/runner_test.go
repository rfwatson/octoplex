package container_test

import (
	"context"
	"testing"
	"time"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/testhelpers"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)
	defer apiClient.Close()

	logger := testhelpers.NewTestLogger()
	containerName := "termstream-test-" + uuid.NewString()
	component := "test-start-stop"

	runner, err := container.NewRunner(logger)
	require.NoError(t, err)

	running, err := runner.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	_, err = runner.RunContainer(ctx, container.RunContainerParams{
		Name:        containerName,
		Image:       "bluenviron/mediamtx",
		Labels:      map[string]string{"component": component},
		NetworkMode: "default",
	})
	require.NoError(t, err)

	require.Eventually(
		t,
		func() bool {
			running, err = runner.ContainerRunning(ctx, map[string]string{"component": component})
			return err == nil && running
		},
		5*time.Second,
		250*time.Millisecond,
		"container not in RUNNING state",
	)

	runner.Close()

	running, err = runner.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)
}
