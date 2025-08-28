//go:build integration

package container_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/shortid"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationClientStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger(t)
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	containerName := "octoplex-test-" + shortid.New().String()
	component := "test-start-stop"

	client, err := container.NewClient(ctx, container.NewParams{APIClient: apiClient, Logger: logger})
	require.NoError(t, err)

	running, err := client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)

	containerStateC, errC := client.RunContainer(ctx, container.RunContainerParams{
		Name:     containerName,
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "ghcr.io/rfwatson/mediamtx-alpine:latest",
			Labels: map[string]string{container.LabelComponent: component},
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
			running, err = client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
			return err == nil && running
		},
		5*time.Second,
		100*time.Millisecond,
		"container not in RUNNING state",
	)

	client.Close()
	require.NoError(t, <-errC)

	running, err = client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)
}

func TestIntegrationClientRemoveContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger(t)
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	component := "test-remove-containers"

	client, err := container.NewClient(ctx, container.NewParams{APIClient: apiClient, Logger: logger})
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	stateC, err1C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "ghcr.io/rfwatson/mediamtx-alpine:latest",
			Labels: map[string]string{container.LabelComponent: component, "group": "test1"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	stateC, err2C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "ghcr.io/rfwatson/mediamtx-alpine:latest",
			Labels: map[string]string{container.LabelComponent: component, "group": "test1"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	stateC, err3C := client.RunContainer(ctx, container.RunContainerParams{
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:  "ghcr.io/rfwatson/mediamtx-alpine:latest",
			Labels: map[string]string{container.LabelComponent: component, "group": "test2"},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(stateC)

	// check all containers in group 1 are running
	require.Eventually(
		t,
		func() bool {
			running, _ := client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{"group": "test1"}))
			return running
		},
		5*time.Second,
		500*time.Millisecond,
		"container group 1 not in RUNNING state",
	)
	// check all containers in group 2 are running
	require.Eventually(
		t,
		func() bool {
			running, _ := client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{"group": "test2"}))
			return running
		},
		2*time.Second,
		500*time.Millisecond,
		"container group 2 not in RUNNING state",
	)

	// remove group 1
	err = client.RemoveContainers(ctx, client.ContainersWithLabels(map[string]string{"group": "test1"}))
	require.NoError(t, err)

	// check group 1 is not running
	require.Eventually(
		t,
		func() bool {
			var running bool
			running, err = client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{"group": "test1"}))
			return err == nil && !running
		},
		2*time.Second,
		500*time.Millisecond,
		"container group 1 still in RUNNING state",
	)

	// check group 2 is still running
	running, err := client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{"group": "test2"}))
	require.NoError(t, err)
	assert.True(t, running)

	assert.NoError(t, <-err1C)
	assert.NoError(t, <-err2C)

	client.Close()

	assert.NoError(t, <-err3C)
}

func TestIntegrationContainerRestart(t *testing.T) {
	const wantRestartCount = 3

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger(t)
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	containerName := "octoplex-test-" + shortid.New().String()
	component := "test-restart"

	client, err := container.NewClient(ctx, container.NewParams{APIClient: apiClient, Logger: logger})
	require.NoError(t, err)
	defer client.Close()

	containerStateC, errC := client.RunContainer(ctx, container.RunContainerParams{
		Name:     containerName,
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Image:      "alpine:3.18",
			Entrypoint: []string{"sleep", "1"},
			Cmd:        []string{"1"}, // 1 second
			Labels:     map[string]string{container.LabelComponent: component},
		},
		HostConfig: &typescontainer.HostConfig{
			NetworkMode: "default",
		},
		ShouldRestart: func(_ int64, restartCount int, _ [][]byte, _ time.Duration) (bool, error) {
			return restartCount < wantRestartCount, nil
		},
		RestartInterval: 1 * time.Second,
	})
	testhelpers.ChanRequireNoError(t, errC)

outer:
	for {
		select {
		case containerState := <-containerStateC:
			if containerState.Status == "running" {
				break outer
			}
		case <-time.After(5 * time.Second):
			require.Fail(t, "timeout waiting for container")
		}
	}

	err = nil // reset error
	done := make(chan struct{})
	go func() {
		defer close(done)

		for {
			containerState := <-containerStateC
			if containerState.Status == domain.ContainerStatusExited {
				if containerState.RestartCount != wantRestartCount {
					err = fmt.Errorf("expected %d restarts, got %d", wantRestartCount, containerState.RestartCount)
				}
				break
			}
		}
	}()

	<-done
	require.NoError(t, err)
}

func TestIntegrationSanitizeLogs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	containerName := "octoplex-test-" + shortid.New().String()
	component := "test-sanitize-logs"

	client, err := container.NewClient(ctx, container.NewParams{APIClient: apiClient, Logger: logger})
	require.NoError(t, err)

	running, err := client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)

	containerStateC, errC := client.RunContainer(ctx, container.RunContainerParams{
		Name:     containerName,
		ChanSize: 1,
		ContainerConfig: &typescontainer.Config{
			Cmd:    []string{"/bin/sh", "-c", "echo 'Connecting to rtmp://rtmp.example.com/s3cr3t...' && sleep 1"},
			Image:  "alpine:latest",
			Labels: map[string]string{container.LabelComponent: component},
		},
		HostConfig: &typescontainer.HostConfig{NetworkMode: "default"},
		Logs: container.LogConfig{
			Stdout: true,
			Sanitizer: func(line []byte) []byte {
				return bytes.ReplaceAll(line, []byte("s3cr3t"), []byte("*****"))
			},
		},
	})
	testhelpers.ChanDiscard(containerStateC)
	testhelpers.ChanRequireNoError(t, errC)

	time.Sleep(time.Second)

	client.Close()
	require.NoError(t, <-errC)

	running, err = client.ContainerRunning(ctx, client.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)

	assert.Contains(t, logs.String(), "Connecting to rtmp://rtmp.example.com/*****")
}
