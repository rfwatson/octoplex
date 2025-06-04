//go:build integration

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationCLI(t *testing.T) {
	done := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer func() {
		cancel()

		<-done
	}()

	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")

	go func() {
		_, _, err := runCommand(ctx, t, "octoplex", "server", "start", "--data-dir", dataDir, "--listen-addr", ":3001")
		assert.ErrorIs(t, err, context.Canceled)
		done <- struct{}{}
	}()

	time.Sleep(2 * time.Second) // Give the server time to start

	stdout, _, err := runCommand(ctx, t, "octoplex", "client", "--host", "localhost:3001", "--tls-skip-verify", "destination", "add", "--name", "local server", "--url", "rtmp://localhost:1935/live")
	destinationID := chomp(stdout)
	require.NoError(t, err)
	require.Len(t, destinationID, 36) // Check if the ID is a valid UUID length

	stdout, _, err = runCommand(ctx, t, "octoplex", "client", "--host", "localhost:3001", "--tls-skip-verify", "destination", "update", destinationID, "--name", "my new name")
	require.NoError(t, err)
	assert.Equal(t, "OK", chomp(stdout))

	_, _, err = runCommand(ctx, t, "octoplex", "client", "--host", "localhost:3001", "--tls-skip-verify", "destination", "start", "--id", destinationID)
	assert.EqualError(t, err, "start destination: start destination failed: source not live")

	stdout, _, err = runCommand(ctx, t, "octoplex", "client", "--host", "localhost:3001", "--tls-skip-verify", "destination", "remove", destinationID)
	require.NoError(t, err)
	assert.Equal(t, "OK", chomp(stdout))

	_, _, err = runCommand(ctx, t, "octoplex", "client", "--host", "localhost:3001", "--tls-skip-verify", "destination", "start", "--id", destinationID)
	assert.EqualError(t, err, "start destination: start destination failed: destination not found")
}

func runCommand(ctx context.Context, _ *testing.T, args ...string) (string, string, error) {
	var stdout, stderr concurrentBuffer

	err := run(ctx, &stdout, &stderr, args)
	if err != nil {
		return "", "", err
	}

	return stdout.String(), stderr.String(), nil
}

func chomp(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}
