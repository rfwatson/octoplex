package mediaserver_test

import (
	"context"
	"testing"
	"time"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/mediaserver"
	"git.netflux.io/rob/termstream/testhelpers"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const component = "mediaserver"

func TestMediaServerStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger()
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	containerClient, err := container.NewClient(ctx, apiClient, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, containerClient.Close()) })

	running, err := containerClient.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	mediaServer := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		FetchIngressStateInterval: 500 * time.Millisecond,
		ChanSize:                  1,
		ContainerClient:           containerClient,
		Logger:                    logger,
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(mediaServer.C())

	require.Eventually(
		t,
		func() bool {
			running, err = containerClient.ContainerRunning(ctx, map[string]string{"component": component})
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
			return currState.Container.RxRate > 500
		},
		time.Second*10,
		time.Second,
		"actor not healthy and/or in LIVE state",
	)

	mediaServer.Close()

	running, err = containerClient.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)
}
