//go:build integration

package app_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/app"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/container/mocks"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

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

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	logger.Info("Initialised logger", "debug_level", logger.Enabled(ctx, slog.LevelDebug), "runner_debug", os.Getenv("RUNNER_DEBUG"))
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	// https://stackoverflow.com/a/60740997/62871
	if runtime.GOOS != "linux" {
		panic("TODO: try host.docker.internal or Mac equivalent here")
	}
	const destHost = "172.17.0.1"

	destURL1 := fmt.Sprintf("rtmp://%s:%d/live/dest1", destHost, destServerPort.Int())
	destURL2 := fmt.Sprintf("rtmp://%s:%d/live/dest2", destHost, destServerPort.Int())
	configService := setupConfigService(t, config.Config{
		Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true, StreamKey: "live"}},
		// Load one destination from config, add the other in-app.
		Destinations: []config.Destination{{Name: "Local server 1", URL: destURL1}},
	})

	done := make(chan struct{})
	go func() {
		defer func() {
			done <- struct{}{}
		}()

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
	}()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   waiting for stream", "expected mediaserver status to be waiting")
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

	sendKey(screen, tcell.KeyDelete, ' ')
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

func TestIntegrationDestinationValidations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	configService := setupConfigService(t, config.Config{
		Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true, StreamKey: "live"}},
	})

	done := make(chan struct{})
	go func() {
		defer func() {
			done <- struct{}{}
		}()

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
	}()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   waiting for stream", "expected mediaserver status to be waiting")
			assert.True(t, contentsIncludes(contents, "No destinations added yet. Press [a] to add a new destination."), "expected to see no destinations message")
		},
		2*time.Minute,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(getContents, "After starting the mediaserver")

	sendKey(screen, tcell.KeyRune, 'a')
	sendKey(screen, tcell.KeyTab, ' ')
	sendBackspaces(screen, 10)
	sendKey(screen, tcell.KeyTab, ' ')
	sendKey(screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()

			assert.True(t, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(t, contentsIncludes(contents, "validate: destination URL must start with"), "expected to see config update error")
		},
		10*time.Second,
		time.Second,
		"expected a validation error for an empty URL",
	)
	printScreen(getContents, "After entering an empty destination URL")

	sendKey(screen, tcell.KeyEnter, ' ')
	sendKey(screen, tcell.KeyBacktab, ' ')
	sendKeys(screen, "nope")
	sendKey(screen, tcell.KeyTab, ' ')
	sendKey(screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()

			assert.True(t, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(t, contentsIncludes(contents, "validate: destination URL must start with"), "expected to see config update error")
		},
		10*time.Second,
		time.Second,
		"expected a validation error for an invalid URL",
	)
	printScreen(getContents, "After entering an invalid destination URL")

	sendKey(screen, tcell.KeyEnter, ' ')
	sendKey(screen, tcell.KeyBacktab, ' ')
	sendBackspaces(screen, len("nope"))
	sendKeys(screen, "rtmp://rtmp.youtube.com:1935/live")
	sendKey(screen, tcell.KeyTab, ' ')
	sendKey(screen, tcell.KeyEnter, ' ')

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			require.Contains(t, contents[2], "My stream", "expected new destination to be present")
			assert.Contains(t, contents[2], "off-air", "expected new destination to be off-air")
			assert.False(t, contentsIncludes(contents, "No destinations added yet. Press [a] to add a new destination."), "expected to not see no destinations message")
		},
		10*time.Second,
		time.Second,
		"expected to add the destination",
	)
	printScreen(getContents, "After adding the destination")

	sendKey(screen, tcell.KeyRune, 'a')
	sendKey(screen, tcell.KeyTab, ' ')
	sendBackspaces(screen, 10)
	sendKeys(screen, "rtmp://rtmp.youtube.com:1935/live")
	sendKey(screen, tcell.KeyTab, ' ')
	sendKey(screen, tcell.KeyEnter, ' ')

	// Start streaming a test video to the app:
	testhelpers.StreamFLV(t, "rtmp://localhost:1935/live")

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()

			assert.True(t, contentsIncludes(contents, "Configuration update failed:"), "expected to see config update error")
			assert.True(t, contentsIncludes(contents, "validate: duplicate destination URL: rtmp://"), "expected to see config update error")
		},
		10*time.Second,
		time.Second,
		"expected a validation error for a duplicate URL",
	)
	printScreen(getContents, "After entering a duplicate destination URL")
	cancel()

	<-done
}

func TestIntegrationStartupCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	// Start a container that looks like a stale Octoplex container:
	staleContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "bluenviron/mediamtx:latest",
			Env:          map[string]string{"MTX_RTMPADDRESS": ":1937"},
			ExposedPorts: []string{"1937/tcp"},
			WaitingFor:   wait.ForListeningPort("1937/tcp"),
			// It doesn't matter what image this container uses, as long as it has
			// labels that look like an Octoplex container:
			Labels: map[string]string{container.LabelApp: domain.AppName},
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, staleContainer)
	require.NoError(t, err)

	require.NoError(t, err)
	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	configService := setupConfigService(t, config.Config{Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true}}})
	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	done := make(chan struct{})
	go func() {
		defer func() {
			done <- struct{}{}
		}()

		err := app.Run(ctx, app.RunParams{
			ConfigService: configService,
			DockerClient:  dockerClient,
			Screen: &terminal.Screen{
				Screen:   screen,
				Width:    200,
				Height:   25,
				CaptureC: screenCaptureC,
			},
			ClipboardAvailable: false,
			BuildInfo:          domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
			Logger:             logger,
		})
		require.NoError(t, err)
	}()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			assert.True(t, contentsIncludes(getContents(), "Another instance of Octoplex may already be running."), "expected to see startup check modal")
		},
		30*time.Second,
		time.Second,
		"expected to see startup check modal",
	)
	printScreen(getContents, "Ater displaying the startup check modal")

	sendKey(screen, tcell.KeyEnter, ' ') // quit other containers

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			_, err := staleContainer.State(context.Background())
			// IsRunning() does not work, probably because we're undercutting the
			// testcontainers API.
			require.True(t, errdefs.IsNotFound(err), "expected to not find the container")
		},
		time.Minute,
		2*time.Second,
		"expected to quit the other containers",
	)
	printScreen(getContents, "After quitting the other containers")

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			contents := getContents()
			require.True(t, len(contents) > 2, "expected at least 3 lines of output")

			assert.Contains(t, contents[2], "Status   waiting for stream", "expected mediaserver status to be waiting")
		},
		10*time.Second,
		time.Second,
		"expected the mediaserver to start",
	)
	printScreen(getContents, "After starting the mediaserver")
	cancel()

	<-done
}

func TestIntegrationMediaServerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	lis, err := net.Listen("tcp", ":1935")
	require.NoError(t, err)
	t.Cleanup(func() { lis.Close() })

	logger := testhelpers.NewTestLogger(t).With("component", "integration")
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	configService := setupConfigService(t, config.Config{Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true}}})
	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	done := make(chan struct{})
	go func() {
		defer func() {
			done <- struct{}{}
		}()

		err := app.Run(ctx, app.RunParams{
			ConfigService: configService,
			DockerClient:  dockerClient,
			Screen: &terminal.Screen{
				Screen:   screen,
				Width:    200,
				Height:   25,
				CaptureC: screenCaptureC,
			},
			ClipboardAvailable: false,
			BuildInfo:          domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
			Logger:             logger,
		})
		require.NoError(t, err)
	}()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			assert.True(t, contentsIncludes(getContents(), "Mediaserver error: Server process exited unexpectedly."), "expected to see title")
			assert.True(t, contentsIncludes(getContents(), "address already in use"), "expected to see message")
		},
		time.Minute,
		time.Second,
		"expected to see media server error modal",
	)
	printScreen(getContents, "Ater displaying the media server error modal")

	// Quit the app, this should cause the done channel to receive.
	sendKey(screen, tcell.KeyEnter, ' ')

	<-done
}

func TestIntegrationDockerClientError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	logger := testhelpers.NewTestLogger(t).With("component", "integration")

	var dockerClient mocks.DockerClient
	dockerClient.EXPECT().NetworkCreate(mock.Anything, mock.Anything, mock.Anything).Return(network.CreateResponse{}, errors.New("boom"))

	configService := setupConfigService(t, config.Config{Sources: config.Sources{RTMP: config.RTMPSource{Enabled: true}}})
	screen, screenCaptureC, getContents := setupSimulationScreen(t)

	done := make(chan struct{})
	go func() {
		defer func() {
			done <- struct{}{}
		}()

		err := app.Run(ctx, app.RunParams{
			ConfigService: configService,
			DockerClient:  &dockerClient,
			Screen: &terminal.Screen{
				Screen:   screen,
				Width:    200,
				Height:   25,
				CaptureC: screenCaptureC,
			},
			ClipboardAvailable: false,
			BuildInfo:          domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
			Logger:             logger,
		})
		require.EqualError(t, err, "create container client: network create: boom")
	}()

	require.EventuallyWithT(
		t,
		func(t *assert.CollectT) {
			assert.True(t, contentsIncludes(getContents(), "An error occurred:"), "expected to see error message")
			assert.True(t, contentsIncludes(getContents(), "create container client: network create: boom"), "expected to see message")
		},
		5*time.Second,
		time.Second,
		"expected to see fatal error modal",
	)
	printScreen(getContents, "Ater displaying the fatal error modal")

	// Quit the app, this should cause the done channel to receive.
	sendKey(screen, tcell.KeyEnter, ' ')

	<-done
}
