//go:build integration

package app_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
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
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	configService, err := config.NewDefaultService()
	require.NoError(t, err)

	destServer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bluenviron/mediamtx:latest",
			Env:          map[string]string{"MTX_RTMPADDRESS": ":1936"},
			ExposedPorts: []string{"1936/tcp"},
			WaitingFor:   wait.ForListeningPort("1936/tcp"),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, destServer)
	require.NoError(t, err)
	destServerPort, err := destServer.MappedPort(ctx, "1936/tcp")
	require.NoError(t, err)

	logger := testhelpers.NewTestLogger().With("component", "integration")
	logger.Info("Initialised logger", "debug_level", logger.Enabled(ctx, slog.LevelDebug), "runner_debug", os.Getenv("RUNNER_DEBUG"))
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

	t.Cleanup(func() {
		if t.Failed() {
			printScreen(getContents, "After failing")
		}
	})

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

	// https://stackoverflow.com/a/60740997/62871
	if runtime.GOOS != "linux" {
		panic("TODO: try host.docker.internal or Mac equivalent here")
	}
	const destHost = "172.17.0.1"

	destURL1 := fmt.Sprintf("rtmp://%s:%d/live/dest1", destHost, destServerPort.Int())
	destURL2 := fmt.Sprintf("rtmp://%s:%d/live/dest2", destHost, destServerPort.Int())
	cfg := config.Config{
		Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true, StreamKey: "live"}},
		// Load one destination from config, add the other in-app.
		Destinations: []config.Destination{{Name: "Local server 1", URL: destURL1}},
	}

	tmpDir, err := os.MkdirTemp("", "octoplex-app-test-integration")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	configService, err = config.NewService(func() (string, error) { return tmpDir, nil }, 1)
	require.NoError(t, err)
	require.NoError(t, configService.SetConfig(cfg))

	done := make(chan struct{})
	go func() {
		err := app.Run(ctx, app.RunParams{
			ConfigService: configService,
			DockerClient:  dockerClient,
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

	time.Sleep(5 * time.Second)
	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   ready", "expected mediaserver status to be ready")
		},
		2*time.Minute,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(getContents, "After starting the mediaserver")

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(t, contents[3], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(t, contents[4], "Health   healthy", "expected mediaserver to be healthy")
		},
		time.Minute,
		time.Second,
		"expected to receive an ingress stream",
	)
	printScreen(getContents, "After receiving the ingress stream")

	// Add a second destination in-app:
	sendKey(screen, tcell.KeyRune, 'a')

	sendBackspaces(screen, 30)
	sendKeys(screen, "Local server 2")
	sendKey(screen, tcell.KeyTab, ' ')

	sendBackspaces(screen, 30)
	sendKeys(screen, destURL2)
	sendKey(screen, tcell.KeyTab, ' ')
	sendKey(screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(t, contents[3], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(t, contents[4], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(t, contents[2], "off-air", "expected local server 1 to be off-air")

			require.Contains(t, contents[3], "Local server 2", "expected local server 2 to be present")
			assert.Contains(t, contents[3], "off-air", "expected local server 2 to be off-air")

		},
		2*time.Minute,
		time.Second,
		"expected to add the destinations",
	)
	printScreen(getContents, "After adding the destinations")

	// Start destinations:
	sendKey(screen, tcell.KeyRune, ' ')
	sendKey(screen, tcell.KeyDown, ' ')
	sendKey(screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(t, contents[3], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(t, contents[4], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(t, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(t, contents[2], "healthy", "expected local server 1 to be healthy")

			require.Contains(t, contents[3], "Local server 2", "expected local server 2 to be present")
			assert.Contains(t, contents[3], "sending", "expected local server 2 to be sending")
			assert.Contains(t, contents[3], "healthy", "expected local server 2 to be healthy")
		},
		2*time.Minute,
		time.Second,
		"expected to start the destination streams",
	)
	printScreen(getContents, "After starting the destination streams")

	sendKey(screen, tcell.KeyRune, 'r')
	sendKey(screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   receiving", "expected mediaserver status to be receiving")
			assert.Contains(t, contents[3], "Tracks   H264", "expected mediaserver tracks to be H264")
			assert.Contains(t, contents[4], "Health   healthy", "expected mediaserver to be healthy")

			require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(t, contents[2], "sending", "expected local server 1 to be sending")
			assert.Contains(t, contents[2], "healthy", "expected local server 1 to be healthy")

			require.NotContains(t, contents[3], "Local server 2", "expected local server 2 to not be present")

		},
		2*time.Minute,
		time.Second,
		"expected to remove the second destination",
	)
	printScreen(getContents, "After removing the second destination")

	// Stop remaining destination.
	// It is currently necessary to press down to re-focus the destination:
	sendKey(screen, tcell.KeyDown, ' ')
	sendKey(screen, tcell.KeyRune, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			require.Contains(t, contents[2], "Local server 1", "expected local server 1 to be present")
			assert.Contains(t, contents[2], "exited", "expected local server 1 to have exited")

			require.NotContains(t, contents[3], "Local server 2", "expected local server 2 to not be present")
		},
		time.Minute,
		time.Second,
		"expected to stop the first destination stream",
	)

	printScreen(getContents, "After stopping the first destination")

	// TODO:
	// - Source error
	// - Destination error
	// - Additional features (copy URL, etc.)

	cancel()

	<-done
}

func printScreen(getContents func() []string, label string) {
	fmt.Println(label + ":")
	for _, line := range getContents() {
		fmt.Println(line)
	}
}

func sendKey(screen tcell.SimulationScreen, key tcell.Key, ch rune) {
	screen.InjectKey(key, ch, tcell.ModNone)
	time.Sleep(50 * time.Millisecond)
}
func sendKeys(screen tcell.SimulationScreen, keys string) {
	screen.InjectKeyBytes([]byte(keys))
	time.Sleep(500 * time.Millisecond)
}

func sendBackspaces(screen tcell.SimulationScreen, n int) {
	for range n {
		screen.InjectKey(tcell.KeyBackspace, ' ', tcell.ModNone)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}
