package testhelpers

import (
	"context"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// StreamFLV streams a test video to the given URL.
func StreamFLV(t *testing.T, destURL string) {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-r", "30",
		"-f", "lavfi",
		"-i", "testsrc",
		"-vf", "scale=1280:960",
		"-vcodec", "libx264",
		"-x264-params", "keyint=30:scenecut=0", // 1 key frame per second
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
}
