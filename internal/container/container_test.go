package container_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/container"
	containermocks "git.netflux.io/rob/octoplex/internal/generated/mocks/container"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestClientRunContainer(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	// channels returned by Docker's ContainerWait:
	containerWaitC := make(chan dockercontainer.WaitResponse)
	containerErrC := make(chan error)

	// channels returned by Docker's Events:
	eventsC := make(chan events.Message)
	eventsErrC := make(chan error)

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		NetworkCreate(mock.Anything, mock.Anything, mock.MatchedBy(func(opts network.CreateOptions) bool {
			return opts.Driver == "bridge" && len(opts.Labels) > 0
		})).
		Return(network.CreateResponse{ID: "test-network"}, nil)
	dockerClient.
		EXPECT().
		ImagePull(mock.Anything, "alpine", image.PullOptions{}).
		Return(nil, errors.New("error pulling image should not be fatal"))
	dockerClient.
		EXPECT().
		ContainerCreate(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*ocispec.Platform)(nil), mock.Anything).
		Return(dockercontainer.CreateResponse{ID: "123"}, nil)
	dockerClient.
		EXPECT().
		NetworkConnect(mock.Anything, "test-network", "123", (*network.EndpointSettings)(nil)).
		Return(nil)
	dockerClient.
		EXPECT().
		CopyToContainer(mock.Anything, "123", "/", mock.Anything, dockercontainer.CopyToContainerOptions{}).
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerStart(mock.Anything, "123", dockercontainer.StartOptions{}).
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerStats(mock.Anything, "123", true).
		Return(dockercontainer.StatsResponseReader{Body: io.NopCloser(bytes.NewReader(nil))}, nil)
	dockerClient.
		EXPECT().
		ContainerWait(mock.Anything, "123", dockercontainer.WaitConditionNextExit).
		Return(containerWaitC, containerErrC)
	dockerClient.
		EXPECT().
		Events(mock.Anything, events.ListOptions{Filters: filters.NewArgs(filters.Arg("container", "123"), filters.Arg("type", "container"))}).
		Return(eventsC, eventsErrC)
	dockerClient.
		EXPECT().
		ContainerLogs(mock.Anything, "123", mock.Anything).
		Return(io.NopCloser(bytes.NewReader(nil)), nil)

	containerClient, err := container.NewClient(t.Context(), container.NewParams{APIClient: &dockerClient, Logger: logger})
	require.NoError(t, err)

	containerStateC, errC := containerClient.RunContainer(t.Context(), container.RunContainerParams{
		Name:            "test-run-container",
		ChanSize:        1,
		ContainerConfig: &dockercontainer.Config{Image: "alpine"},
		HostConfig:      &dockercontainer.HostConfig{},
		Logs:            container.LogConfig{Stdout: true},
		CopyFiles: []container.CopyFileConfig{
			{
				Path:    "/hello",
				Payload: bytes.NewReader([]byte("world")),
				Mode:    0755,
			},
			{
				Path:    "/foo/bar",
				Payload: bytes.NewReader([]byte("baz")),
				Mode:    0755,
			},
		},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)

		require.NoError(t, <-errC)
	}()

	assert.Equal(t, "pulling", (<-containerStateC).Status)
	assert.Equal(t, "created", (<-containerStateC).Status)
	assert.Equal(t, "running", (<-containerStateC).Status)
	assert.Equal(t, "running", (<-containerStateC).Status)

	// Enough time for events channel to receive a message:
	time.Sleep(100 * time.Millisecond)

	containerWaitC <- dockercontainer.WaitResponse{StatusCode: 1}

	state := <-containerStateC
	assert.Equal(t, "exited", state.Status)
	assert.Equal(t, "unhealthy", state.HealthState)
	require.NotNil(t, state.ExitCode)
	assert.Equal(t, 1, *state.ExitCode)
	assert.Equal(t, 0, state.RestartCount)

	<-done
}

func TestClientRunContainerWithRestart(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	// channels returned by Docker's ContainerWait:
	containerWaitC := make(chan dockercontainer.WaitResponse)
	containerErrC := make(chan error)

	// channels returned by Docker's Events:
	eventsC := make(chan events.Message)
	eventsErrC := make(chan error)

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		NetworkCreate(mock.Anything, mock.Anything, mock.MatchedBy(func(opts network.CreateOptions) bool {
			return opts.Driver == "bridge" && len(opts.Labels) > 0
		})).
		Return(network.CreateResponse{ID: "test-network"}, nil)
	dockerClient.
		EXPECT().
		ImagePull(mock.Anything, "alpine", image.PullOptions{}).
		Return(io.NopCloser(bytes.NewReader(nil)), nil)
	dockerClient.
		EXPECT().
		ContainerCreate(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*ocispec.Platform)(nil), mock.Anything).
		Return(dockercontainer.CreateResponse{ID: "123"}, nil)
	dockerClient.
		EXPECT().
		NetworkConnect(mock.Anything, "test-network", "123", (*network.EndpointSettings)(nil)).
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerStart(mock.Anything, "123", dockercontainer.StartOptions{}).
		Once().
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerStats(mock.Anything, "123", true).
		Return(dockercontainer.StatsResponseReader{Body: io.NopCloser(bytes.NewReader(nil))}, nil)
	dockerClient.
		EXPECT().
		ContainerWait(mock.Anything, "123", dockercontainer.WaitConditionNextExit).
		Return(containerWaitC, containerErrC)
	dockerClient.
		EXPECT().
		Events(mock.Anything, events.ListOptions{Filters: filters.NewArgs(filters.Arg("container", "123"), filters.Arg("type", "container"))}).
		Return(eventsC, eventsErrC)
	dockerClient.
		EXPECT().
		ContainerStart(mock.Anything, "123", dockercontainer.StartOptions{}). // restart
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerLogs(mock.Anything, "123", mock.Anything).
		Return(io.NopCloser(bytes.NewReader(nil)), nil)

	containerClient, err := container.NewClient(t.Context(), container.NewParams{APIClient: &dockerClient, Logger: logger})
	require.NoError(t, err)

	containerStateC, errC := containerClient.RunContainer(t.Context(), container.RunContainerParams{
		Name:            "test-run-container",
		ChanSize:        1,
		ContainerConfig: &dockercontainer.Config{Image: "alpine"},
		HostConfig:      &dockercontainer.HostConfig{},
		Logs:            container.LogConfig{Stdout: true},
		ShouldRestart: func(_ int64, restartCount int, _ [][]byte, _ time.Duration) (bool, error) {
			if restartCount == 0 {
				return true, nil
			}

			return false, errors.New("max restarts reached")
		},
		RestartInterval: 10 * time.Millisecond,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)

		require.NoError(t, <-errC)
	}()

	assert.Equal(t, "pulling", (<-containerStateC).Status)
	assert.Equal(t, "created", (<-containerStateC).Status)
	assert.Equal(t, "running", (<-containerStateC).Status)
	assert.Equal(t, "running", (<-containerStateC).Status)

	// Enough time for the restart to occur:
	time.Sleep(100 * time.Millisecond)

	containerWaitC <- dockercontainer.WaitResponse{StatusCode: 1}

	state := <-containerStateC
	assert.Equal(t, "restarting", state.Status)
	assert.Equal(t, "unhealthy", state.HealthState)
	assert.Nil(t, state.ExitCode)
	assert.Zero(t, state.RestartCount) // not incremented until the actual restart

	// During the restart, the "running" status is triggered by Docker events
	// only. So we don't expect one in unit tests. (Probably the initial startup
	// flow should behave the same.)

	time.Sleep(100 * time.Millisecond)
	containerWaitC <- dockercontainer.WaitResponse{StatusCode: 1}

	state = <-containerStateC
	assert.Equal(t, "exited", state.Status)
	assert.Equal(t, "unhealthy", state.HealthState)
	require.NotNil(t, state.ExitCode)
	assert.Equal(t, 1, *state.ExitCode)
	assert.Equal(t, 1, state.RestartCount)
	assert.Equal(t, "max restarts reached", state.Err.Error())

	<-done
}

func TestClientRunContainerErrorStartingContainer(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		NetworkCreate(mock.Anything, mock.Anything, mock.MatchedBy(func(opts network.CreateOptions) bool {
			return opts.Driver == "bridge" && len(opts.Labels) > 0
		})).
		Return(network.CreateResponse{ID: "test-network"}, nil)
	dockerClient.
		EXPECT().
		ImagePull(mock.Anything, "alpine", image.PullOptions{}).
		Return(io.NopCloser(bytes.NewReader(nil)), nil)
	dockerClient.
		EXPECT().
		ContainerCreate(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*ocispec.Platform)(nil), mock.Anything).
		Return(dockercontainer.CreateResponse{ID: "123"}, nil)
	dockerClient.
		EXPECT().
		NetworkConnect(mock.Anything, "test-network", "123", (*network.EndpointSettings)(nil)).
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerStart(mock.Anything, "123", dockercontainer.StartOptions{}).
		Return(errors.New("error starting container"))

	containerClient, err := container.NewClient(t.Context(), container.NewParams{APIClient: &dockerClient, Logger: logger})
	require.NoError(t, err)

	containerStateC, errC := containerClient.RunContainer(t.Context(), container.RunContainerParams{
		Name:            "test-run-container-error-starting",
		ChanSize:        1,
		ContainerConfig: &dockercontainer.Config{Image: "alpine"},
		HostConfig:      &dockercontainer.HostConfig{},
	})

	assert.Equal(t, "pulling", (<-containerStateC).Status)
	assert.Equal(t, "created", (<-containerStateC).Status)

	err = <-errC
	require.EqualError(t, err, "container start: error starting container")
}

func TestClientClose(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		NetworkCreate(mock.Anything, mock.Anything, mock.MatchedBy(func(opts network.CreateOptions) bool {
			return opts.Driver == "bridge" && len(opts.Labels) > 0
		})).
		Return(network.CreateResponse{ID: "test-network"}, nil)
	dockerClient.
		EXPECT().
		ContainerList(mock.Anything, mock.Anything).
		Return([]dockercontainer.Summary{{ID: "123"}}, nil)
	dockerClient.
		EXPECT().
		ContainerStop(mock.Anything, "123", mock.Anything).
		Return(nil)
	dockerClient.
		EXPECT().
		ContainerRemove(mock.Anything, "123", mock.Anything).
		Return(nil)
	dockerClient.
		EXPECT().
		NetworkRemove(mock.Anything, "test-network").
		Return(nil)
	dockerClient.
		EXPECT().
		Close().
		Return(nil)

	containerClient, err := container.NewClient(t.Context(), container.NewParams{APIClient: &dockerClient, Logger: logger})
	require.NoError(t, err)

	require.NoError(t, containerClient.Close())
}

func TestRemoveUnusedNetworks(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		NetworkCreate(mock.Anything, mock.Anything, mock.MatchedBy(func(opts network.CreateOptions) bool {
			return opts.Driver == "bridge" && len(opts.Labels) > 0
		})).
		Return(network.CreateResponse{ID: "test-network"}, nil)
	dockerClient.
		EXPECT().
		NetworkList(mock.Anything, mock.Anything).
		Return([]network.Summary{
			{ID: "test-network"},
			{ID: "another-network"},
		}, nil)
	dockerClient.
		EXPECT().
		NetworkRemove(mock.Anything, "another-network").
		Return(nil)

	containerClient, err := container.NewClient(t.Context(), container.NewParams{APIClient: &dockerClient, Logger: logger})
	require.NoError(t, err)

	require.NoError(t, containerClient.RemoveUnusedNetworks(t.Context()))
}
