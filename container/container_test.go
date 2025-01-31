package container_test

import (
	"context"
	"testing"
	"time"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/testhelpers"
	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger()
	containerName := "termstream-test-" + uuid.NewString()
	component := "test-start-stop"

	client, err := container.NewClient(ctx, logger)
	require.NoError(t, err)

	running, err := client.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	containerStateC, errC := client.RunContainer(ctx, container.RunContainerParams{
		Name:     containerName,
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "netfluxio/mediamtx-alpine:latest",
			Labels: map[string]string{"component": component},
		},
		HostConfig: &typescontainer.HostConfig{
			NetworkMode: "default",
		},
	})
	testhelpers.ChanDiscard(containerStateC)
	testhelpers.ChanRequireNoError(t, errC)

	require.Eventually(
		t,
		func() bool {
			running, err = client.ContainerRunning(ctx, map[string]string{"component": component})
			return err == nil && running
		},
		5*time.Second,
		100*time.Millisecond,
		"container not in RUNNING state",
	)

	client.Close()
	require.NoError(t, <-errC)

	running, err = client.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)
}

func TestClientRemoveContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger()
	component := "test-remove-containers"

	client, err := container.NewClient(ctx, logger)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	stateC, err1C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "netfluxio/mediamtx-alpine:latest",
			Labels: map[string]string{"component": component, "group": "test1"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	stateC, err2C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "netfluxio/mediamtx-alpine:latest",
			Labels: map[string]string{"component": component, "group": "test1"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	stateC, err3C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "netfluxio/mediamtx-alpine:latest",
			Labels: map[string]string{"component": component, "group": "test2"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	// check all containers in group 1 are running
	require.Eventually(
		t,
		func() bool {
			running, _ := client.ContainerRunning(ctx, map[string]string{"group": "test1"})
			return running
		},
		5*time.Second,
		100*time.Millisecond,
		"container group 1 not in RUNNING state",
	)
	// check all containers in group 2 are running
	require.Eventually(
		t,
		func() bool {
			running, _ := client.ContainerRunning(ctx, map[string]string{"group": "test2"})
			return running
		},
		2*time.Second,
		100*time.Millisecond,
		"container group 2 not in RUNNING state",
	)

	// remove group 1
	err = client.RemoveContainers(ctx, map[string]string{"group": "test1"})
	require.NoError(t, err)

	// check group 1 is not running
	require.Eventually(
		t,
		func() bool {
			var running bool
			running, err = client.ContainerRunning(ctx, map[string]string{"group": "test1"})
			return err == nil && !running
		},
		2*time.Second,
		100*time.Millisecond,
		"container group 1 still in RUNNING state",
	)

	// check group 2 is still running
	running, err := client.ContainerRunning(ctx, map[string]string{"group": "test2"})
	require.NoError(t, err)
	assert.True(t, running)

	assert.NoError(t, <-err1C)
	assert.NoError(t, <-err2C)

	client.Close()

	assert.NoError(t, <-err3C)
}
