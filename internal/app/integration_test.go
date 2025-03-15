//go:build integration

package app_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/app"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	dockerclient "github.com/docker/docker/client"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger().With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	require.NoError(t, err)

	// Fetching the screen contents is tricky at this level of the test pyramid,
	// because we need to:
	//
	// 1. Somehow capture the screen contents, which is only available via the
	//    tcell.SimulationScreen, and...
	// 2. Do so without triggering data races.
	//
	// We can achieve this by passing a channel into the terminal actor, which
	// will send screen captures after each render. This can be stored locally
	// and asserted against when needed.
	var (
		screenCells []tcell.SimCell
		screenWidth int
		screenMu    sync.Mutex
	)

	getContents := func() []string {
		screenMu.Lock()
		defer screenMu.Unlock()

		var lines []string
		for n, _ := range screenCells {
			y := n / screenWidth

			if y > len(lines)-1 {
				lines = append(lines, "")
			}
			lines[y] += string(screenCells[n].Runes[0])
		}

		return lines
	}

	screen := tcell.NewSimulationScreen("")
	screenCaptureC := make(chan terminal.ScreenCapture, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case capture := <-screenCaptureC:
				screenMu.Lock()
				screenCells = capture.Cells
				screenWidth = capture.Width
				screenMu.Unlock()
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		err := app.Run(ctx, app.RunParams{
			Config: config.Config{
				// We use the mediaserver as the destination server, just because it is
				// reachable from the docker network via mediaserver:1935.
				Destinations: []config.Destination{
					{
						Name: "Local server 1",
						URL:  "rtmp://mediaserver:1935/live/dest1",
					},
					{
						Name: "Local server 2",
						URL:  "rtmp://mediaserver:1935/live/dest2",
					},
				},
			},
			DockerClient: dockerClient,
			Screen: &terminal.Screen{
				Screen:   screen,
				Width:    160,
				Height:   25,
				CaptureC: screenCaptureC,
			},
			ClipboardAvailable: false,
			BuildInfo:          domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
			Logger:             logger,
		})
		require.NoError(t, err)

		done <- struct{}{}
	}()

	// Wait for mediaserver container to start:
	time.Sleep(5 * time.Second)

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")
	time.Sleep(10 * time.Second)

	// Start destinations:
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyDown, ' ', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	time.Sleep(15 * time.Second)

	contents := getContents()
	require.True(t, len(contents) > 2, "expected at least 3 lines of output")

	require.Contains(t, contents[2], "Status   receiving", "expected mediaserver status to be receiving")
	require.Contains(t, contents[3], "Tracks   H264", "expected mediaserver tracks to be H264")
	require.Contains(t, contents[4], "Health   healthy", "expected mediaserver to be healthy")

	require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
	assert.Contains(t, contents[2], "sending", "expected local server 1 to be sending")
	assert.Contains(t, contents[2], "healthy", "expected local server 1 to be healthy")

	require.Contains(t, contents[3], "Local server 2", "expected local server 2 to be present")
	assert.Contains(t, contents[3], "sending", "expected local server 2 to be sending")
	assert.Contains(t, contents[3], "healthy", "expected local server 2 to be healthy")

	// Stop destinations:
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyUp, ' ', tcell.ModNone))
	screen.PostEvent(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	time.Sleep(10 * time.Second)

	contents = getContents()
	require.True(t, len(contents) > 2, "expected at least 3 lines of output")

	require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
	assert.Contains(t, contents[2], "exited", "expected local server 1 to have exited")

	require.Contains(t, contents[3], "Local server 2", "expected local server 2 to be present")
	assert.Contains(t, contents[3], "exited", "expected local server 2 to have exited")

	// TODO:
	// - Source error
	// - Destination error
	// - Additional features (copy URL, etc.)

	cancel()

	<-done
}
