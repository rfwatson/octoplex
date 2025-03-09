//go:build integration

package app_test

import (
	"context"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/app"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	dockerclient "github.com/docker/docker/client"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	logger := testhelpers.NewTestLogger()
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		require.NoError(t, app.Run(ctx, app.RunParams{
			Config:             config.Config{},
			DockerClient:       dockerClient,
			Screen:             tcell.NewSimulationScreen(""),
			ClipboardAvailable: false,
			BuildInfo:          domain.BuildInfo{Version: "0.0.1", GoVersion: "go1.16.3"},
			Logger:             logger,
		}))

		done <- struct{}{}
	}()

	// For now, just launch the app and wait for a few seconds.
	// This is mostly useful to verify there are no obvious data races (when
	// running with -race).
	// See https://github.com/rivo/tview/wiki/Concurrency.
	//
	// TODO: test more user journeys.
	time.Sleep(time.Second * 5)
	cancel()

	<-done
}
