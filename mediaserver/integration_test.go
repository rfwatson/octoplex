//go:build integration

package mediaserver_test

import (
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/container"
	"git.netflux.io/rob/octoplex/mediaserver"
	"git.netflux.io/rob/octoplex/testhelpers"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const component = "mediaserver"

func TestIntegrationMediaServerStartStop(t *testing.T) {
	logger := testhelpers.NewTestLogger()
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	containerClient, err := container.NewClient(t.Context(), apiClient, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, containerClient.Close()) })

	running, err := containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{"component": component}))
	require.NoError(t, err)
	assert.False(t, running)

	mediaServer := mediaserver.StartActor(t.Context(), mediaserver.StartActorParams{
		FetchIngressStateInterval: 500 * time.Millisecond,
		ChanSize:                  1,
		ContainerClient:           containerClient,
		Logger:                    logger,
	})
	require.NoError(t, err)
	t.Cleanup(func() { mediaServer.Close() })
	testhelpers.ChanDiscard(mediaServer.C())

	require.Eventually(
		t,
		func() bool {
			running, err = containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{"component": component}))
			return err == nil && running
		},
		time.Second*10,
		time.Second,
		"container not in RUNNING state",
	)

	state := mediaServer.State()
	assert.False(t, state.Live)
	assert.Equal(t, "rtmp://localhost:1935/live", state.RTMPURL)

	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.Eventually(
		t,
		func() bool {
			currState := mediaServer.State()
			return currState.Live && currState.Container.HealthState == "healthy"
		},
		time.Second*5,
		time.Second,
		"actor not healthy and/or in LIVE state",
	)

	require.Eventually(
		t,
		func() bool {
			currState := mediaServer.State()
			return len(currState.Tracks) == 1 && currState.Tracks[0] == "H264"
		},
		time.Second*5,
		time.Second,
		"tracks not updated",
	)

	require.Eventually(
		t,
		func() bool {
			currState := mediaServer.State()
			return currState.Container.RxRate > 500
		},
		time.Second*10,
		time.Second,
		"rxRate not updated",
	)

	mediaServer.Close()

	running, err = containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{"component": component}))
	require.NoError(t, err)
	assert.False(t, running)
}
