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
	streamFFMPEG(t, destURL, "-f", "flv")
}

// StreamRTSP streams a test video to the given URL.
func StreamRTSP(t *testing.T, destURL string) {
	streamFFMPEG(t, destURL, "-f", "rtsp", "-rtsp_transport", "tcp")
}

func streamFFMPEG(t *testing.T, destURL string, formatFlags ...string) {
	ctx, cancel := context.WithCancel(context.Background())

	args := []string{
		"-r", "30",
		"-f", "lavfi",
		"-i", "testsrc",
		"-vf", "scale=1280:960",
		"-vcodec", "libx264",
		"-x264-params", "keyint=30:scenecut=0", // 1 key frame per second
		"-profile:v", "baseline",
		"-pix_fmt", "yuv420p",
	}

	args = append(args, formatFlags...)
	args = append(args, destURL)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	// Uncomment to view output:
	// cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}

		cancel()
	})
}
