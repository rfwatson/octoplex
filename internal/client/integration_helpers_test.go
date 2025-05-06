//go:build integration

package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/client"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/container"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/server"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"
)

func buildClientServer(
	configService *config.Service,
	dockerClient container.DockerClient,
	screen tcell.SimulationScreen,
	screenCaptureC chan<- terminal.ScreenCapture,
	logger *slog.Logger,
) (*client.App, *server.App) {
	client := client.New(client.NewParams{
		BuildInfo: domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
		Screen: &terminal.Screen{
			Screen:   screen,
			Width:    180,
			Height:   25,
			CaptureC: screenCaptureC,
		},
		Logger: logger,
	})

	server := server.New(server.Params{
		ConfigService: configService,
		DockerClient:  dockerClient,
		WaitForClient: true,
		Logger:        logger,
	})

	return client, server
}

type clientServerResult struct {
	errClient error
	errServer error
}

func runClientServer(
	ctx context.Context,
	_ *testing.T,
	clientApp *client.App,
	serverApp *server.App,
) <-chan clientServerResult {
	ch := make(chan clientServerResult, 1)

	g, ctx := errgroup.WithContext(ctx)
	var clientErr, srvErr error

	g.Go(func() error {
		srvErr = serverApp.Run(ctx)
		return errors.New("server closed")
	})

	g.Go(func() error {
		clientErr = clientApp.Run(ctx)
		return errors.New("client closed")
	})

	go func() {
		_ = g.Wait()

		ch <- clientServerResult{errClient: clientErr, errServer: srvErr}
	}()

	return ch
}

func setupSimulationScreen(t *testing.T) (tcell.SimulationScreen, chan<- terminal.ScreenCapture, func() []string) {
	t.Helper()

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
			if len(screenCells[n].Runes) == 0 { // shouldn't really happen unless there is no output
				continue
			}
			lines[y] += string(screenCells[n].Runes[0])
		}

		require.GreaterOrEqual(t, len(lines), 5, "Screen contents should have at least 5 lines")

		return lines
	}

	t.Cleanup(func() {
		if t.Failed() {
			printScreen(t, getContents, "After failing")
		}
	})

	screen := tcell.NewSimulationScreen("")
	screenCaptureC := make(chan terminal.ScreenCapture, 1)
	go func() {
		for {
			select {
			case <-t.Context().Done():
				return
			case capture := <-screenCaptureC:
				screenMu.Lock()
				screenCells = capture.Cells
				screenWidth = capture.Width
				screenMu.Unlock()
			}
		}
	}()

	return screen, screenCaptureC, getContents
}

func contentsIncludes(contents []string, search string) bool {
	for _, line := range contents {
		if strings.Contains(line, search) {
			return true
		}
	}

	return false
}

func setupConfigService(t *testing.T, cfg config.Config) *config.Service {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "octoplex_"+strings.ReplaceAll(t.Name(), "/", "_"))
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	configService, err := config.NewService(func() (string, error) { return tmpDir, nil }, 1)
	require.NoError(t, err)
	require.NoError(t, configService.SetConfig(cfg))

	return configService
}

func printScreen(t *testing.T, getContents func() []string, label string) {
	t.Helper()

	fmt.Println(label + ":")
	for _, line := range getContents() {
		fmt.Println(line)
	}
}

func sendKey(t *testing.T, screen tcell.SimulationScreen, key tcell.Key, ch rune) {
	t.Helper()

	const (
		waitTime = 50 * time.Millisecond
		maxTries = 50
	)

	for i := 0; i < maxTries; i++ {
		if err := screen.PostEvent(tcell.NewEventKey(key, ch, tcell.ModNone)); err != nil {
			time.Sleep(waitTime)
		} else {
			return
		}
	}

	t.Fatalf("Failed to send key event after %d tries", maxTries)
}

func sendKeys(t *testing.T, screen tcell.SimulationScreen, keys string) {
	t.Helper()

	for _, ch := range keys {
		sendKey(t, screen, tcell.KeyRune, ch)
	}
}

func sendBackspaces(t *testing.T, screen tcell.SimulationScreen, n int) {
	t.Helper()

	for range n {
		sendKey(t, screen, tcell.KeyBackspace, 0)
	}
}

// kickFirstRTMPConn kicks the first RTMP connection from the mediaMTX server.
func kickFirstRTMPConn(t *testing.T, srv testcontainers.Container) {
	t.Helper()

	type conn struct {
		ID string `json:"id"`
	}

	type apiResponse struct {
		Items []conn `json:"items"`
	}

	port, err := srv.MappedPort(t.Context(), "9997/tcp")
	require.NoError(t, err)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/v3/rtmpconns/list", port.Int()))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var apiResp apiResponse
	require.NoError(t, json.Unmarshal(respBody, &apiResp))
	require.NoError(t, err)
	require.True(t, len(apiResp.Items) > 0, "No RTMP connections found")

	resp, err = http.Post(fmt.Sprintf("http://localhost:%d/v3/rtmpconns/kick/%s", port.Int(), apiResp.Items[0].ID), "application/json", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
