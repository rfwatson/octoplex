package container

import (
	"bufio"
	"bytes"
	_ "embed"
	"io"
	"testing"

	containermocks "git.netflux.io/rob/octoplex/internal/generated/mocks/container"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/stats1.json
var statsJSON []byte

//go:embed testdata/stats2.json
var statsWithRestartJSON []byte

func TestHandleStats(t *testing.T) {
	pr, pw := io.Pipe()
	containerID := "b905f51b47242090ae504c184c7bc84d6274511ef763c1847039dcaa00a3ad27"

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		ContainerStats(t.Context(), containerID, true).
		Return(dockercontainer.StatsResponseReader{Body: pr}, nil)

	logger := testhelpers.NewTestLogger(t)
	ch := make(chan stats)

	go func() {
		defer close(ch)

		handleStats(t.Context(), containerID, &dockerClient, logger, ch)
	}()

	go func() {
		defer pw.Close() //nolint:errcheck

		scanner := bufio.NewScanner(bytes.NewReader(statsJSON))
		for scanner.Scan() {
			_, err := pw.Write(scanner.Bytes())
			require.NoError(t, err)
		}
	}()

	var count int
	var lastStats stats
	for stats := range ch {
		count++
		lastStats = stats
	}

	require.Equal(t, 10, count)
	assert.Equal(t, 2.028383838383838, lastStats.cpuPercent)
	assert.Equal(t, uint64(11382784), lastStats.memoryUsageBytes)
	assert.Equal(t, 4186, lastStats.rxRate)
	assert.Equal(t, 217, lastStats.txRate)
}

func TestHandleStatsWithContainerRestart(t *testing.T) {
	pr, pw := io.Pipe()
	containerID := "d0adc747fb12b9ce2376408aed8538a0769de55aa9c239313f231d9d80402e39"

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		ContainerStats(t.Context(), containerID, true).
		Return(dockercontainer.StatsResponseReader{Body: pr}, nil)

	logger := testhelpers.NewTestLogger(t)
	ch := make(chan stats)

	go func() {
		defer close(ch)

		handleStats(t.Context(), containerID, &dockerClient, logger, ch)
	}()

	go func() {
		defer pw.Close() //nolint:errcheck

		scanner := bufio.NewScanner(bytes.NewReader(statsWithRestartJSON))
		for scanner.Scan() {
			_, err := pw.Write(scanner.Bytes())
			require.NoError(t, err)
		}
	}()

	// before container restart:
	var lastStats stats
	for range 14 {
		lastStats = <-ch
	}

	assert.Equal(t, 1.0743624161073824, lastStats.cpuPercent)
	assert.Equal(t, uint64(17223680), lastStats.memoryUsageBytes)
	assert.Equal(t, 2693, lastStats.rxRate)
	assert.Equal(t, 2746, lastStats.txRate)

	// restart triggers zero value stats:
	require.Equal(t, stats{}, <-ch)

	for range 21 {
		lastStats = <-ch
	}

	assert.Equal(t, 2.2858487394957985, lastStats.cpuPercent)
	assert.Equal(t, uint64(13963264), lastStats.memoryUsageBytes)
	assert.Equal(t, 1506, lastStats.rxRate)
	assert.Equal(t, 1532, lastStats.txRate)

	// restart triggers zero value stats:
	require.Equal(t, stats{}, <-ch)

	// after container restart:
	for stats := range ch {
		lastStats = stats
	}

	assert.Equal(t, 1.0191, lastStats.cpuPercent)
	assert.Equal(t, uint64(14540800), lastStats.memoryUsageBytes)
	assert.Equal(t, 2763, lastStats.rxRate)
	assert.Equal(t, 2764, lastStats.txRate)
}
