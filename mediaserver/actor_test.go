package mediaserver_test

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/mediaserver"
	"git.netflux.io/rob/termstream/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const component = "mediaserver"

func TestMediaServerStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	logger := testhelpers.NewTestLogger()
	containerClient, err := container.NewClient(ctx, logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, containerClient.Close()) })

	running, err := containerClient.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	mediaServer := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		ChanSize:        1,
		ContainerClient: containerClient,
		Logger:          logger,
	})
	require.NoError(t, err)
	testhelpers.ChanDiscard(mediaServer.C())

	require.Eventually(
		t,
		func() bool {
			running, err = containerClient.ContainerRunning(ctx, map[string]string{"component": component})
			return err == nil && running
		},
		5*time.Second,
		250*time.Millisecond,
		"container not in RUNNING state",
	)

	state := mediaServer.State()
	assert.False(t, state.Live)
	assert.Equal(t, "rtmp://localhost:1935/live", state.URL)

	launchFFMPEG(t, "rtmp://localhost:1935/live")
	require.Eventually(
		t,
		func() bool {
			currState := mediaServer.State()
			return currState.Live && currState.Container.HealthState == "healthy"
		},
		5*time.Second,
		250*time.Millisecond,
		"actor not healthy and/or in LIVE state",
	)

	mediaServer.Close()

	running, err = containerClient.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)
}

func launchFFMPEG(t *testing.T, destURL string) *exec.Cmd {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-r", "30",
		"-f", "lavfi",
		"-i", "testsrc",
		"-vf", "scale=1280:960",
		"-vcodec", "libx264",
		"-profile:v", "baseline",
		"-pix_fmt", "yuv420p",
		"-f", "flv",
		destURL,
	)

	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}

		cancel()
	})

	return cmd
}
