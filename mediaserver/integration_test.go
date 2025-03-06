//go:build integration

package mediaserver_test

import (
	"fmt"
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

	running, err := containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)

	// We need to avoid clashing with other integration tests, e.g. multiplexer.
	const (
		apiPort  = 9999
		rtmpPort = 1937
	)

	rtmpURL := fmt.Sprintf("rtmp://localhost:%d/live", rtmpPort)

	mediaServer := mediaserver.StartActor(t.Context(), mediaserver.StartActorParams{
		RTMPPort:                  rtmpPort,
		APIPort:                   apiPort,
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
			running, err = containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
			return err == nil && running
		},
		time.Second*10,
		time.Second,
		"container not in RUNNING state",
	)

	state := mediaServer.State()
	assert.False(t, state.Live)
	assert.Equal(t, rtmpURL, state.RTMPURL)

	testhelpers.StreamFLV(t, rtmpURL)

	require.Eventually(
		t,
		func() bool {
			currState := mediaServer.State()
			return currState.Live &&
				!currState.LiveChangedAt.IsZero() &&
				currState.Container.HealthState == "healthy"
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

	running, err = containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)
}
