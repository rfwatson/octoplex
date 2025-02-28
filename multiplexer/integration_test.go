//go:build integration

package multiplexer_test

import (
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/container"
	"git.netflux.io/rob/octoplex/mediaserver"
	"git.netflux.io/rob/octoplex/multiplexer"
	"git.netflux.io/rob/octoplex/testhelpers"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const component = "multiplexer"

func TestIntegrationMultiplexer(t *testing.T) {
	logger := testhelpers.NewTestLogger()
	apiClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)

	containerClient, err := container.NewClient(t.Context(), apiClient, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, containerClient.Close()) })

	running, err := containerClient.ContainerRunning(t.Context(), containerClient.ContainersWithLabels(map[string]string{container.LabelComponent: component}))
	require.NoError(t, err)
	assert.False(t, running)

	srv := mediaserver.StartActor(t.Context(), mediaserver.StartActorParams{
		RTMPPort:                  19350,
		APIPort:                   9998,
		FetchIngressStateInterval: 250 * time.Millisecond,
		ContainerClient:           containerClient,
		ChanSize:                  1,
		Logger:                    logger,
	})
	defer srv.Close()
	testhelpers.ChanDiscard(srv.C())

	time.Sleep(2 * time.Second)
	testhelpers.StreamFLV(t, srv.State().RTMPURL)

	require.Eventually(
		t,
		func() bool { return srv.State().Live },
		time.Second*10,
		time.Second,
		"source not live",
	)

	mp := multiplexer.NewActor(t.Context(), multiplexer.NewActorParams{
		SourceURL:       srv.State().RTMPInternalURL,
		ChanSize:        1,
		ContainerClient: containerClient,
		Logger:          logger,
	})
	defer mp.Close()
	testhelpers.ChanDiscard(mp.C())

	requireListeners(t, srv, 0)

	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test1")
	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test2")
	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test3")
	requireListeners(t, srv, 3)

	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test3")
	requireListeners(t, srv, 2)

	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test2")
	mp.ToggleDestination("rtmp://mediaserver:19350/destination/test1")
	requireListeners(t, srv, 0)
}

func requireListeners(t *testing.T, srv *mediaserver.Actor, expected int) {
	require.Eventually(
		t,
		func() bool { return srv.State().Listeners == expected },
		time.Second*10,
		time.Second,
		"expected %d listeners", expected,
	)
}
