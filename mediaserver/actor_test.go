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
	runner, err := container.NewRunner(logger)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runner.Close()) })

	running, err := runner.ContainerRunning(ctx, map[string]string{"component": component})
	require.NoError(t, err)
	assert.False(t, running)

	actor, err := mediaserver.StartActor(ctx, mediaserver.StartActorParams{
		ChanSize: 1,
		Runner:   runner,
		Logger:   logger,
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

	require.False(t, actor.State().IngressLive)
	launchFFMPEG(t, "rtmp://localhost:1935/live")
	require.Eventually(
		t,
		func() bool { return actor.State().IngressLive },
		5*time.Second,
		250*time.Millisecond,
		"actor not in LIVE state",
	)

	actor.Close()

	running, err = runner.ContainerRunning(ctx, map[string]string{"component": component})
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
