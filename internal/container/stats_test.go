package container

import (
	"bufio"
	"bytes"
	_ "embed"
	"io"
	"testing"

	"git.netflux.io/rob/octoplex/internal/container/mocks"
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

	var dockerClient mocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		ContainerStats(t.Context(), containerID, true).
		Return(dockercontainer.StatsResponseReader{Body: pr}, nil)

	networkCountConfig := NetworkCountConfig{Rx: "eth0", Tx: "eth1"}
	logger := testhelpers.NewTestLogger()
	ch := make(chan stats)

	go func() {
		defer close(ch)

		handleStats(t.Context(), containerID, &dockerClient, networkCountConfig, logger, ch)
	}()

	go func() {
		defer pw.Close()

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
	assert.Equal(t, 4.254369426751593, lastStats.cpuPercent)
	assert.Equal(t, uint64(8802304), lastStats.memoryUsageBytes)
	assert.Equal(t, 1091, lastStats.rxRate)
	assert.Equal(t, 1108, lastStats.txRate)
}

func TestHandleStatsWithContainerRestart(t *testing.T) {
	pr, pw := io.Pipe()
	containerID := "d0adc747fb12b9ce2376408aed8538a0769de55aa9c239313f231d9d80402e39"

	var dockerClient mocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		ContainerStats(t.Context(), containerID, true).
		Return(dockercontainer.StatsResponseReader{Body: pr}, nil)

	networkCountConfig := NetworkCountConfig{Rx: "eth1", Tx: "eth0"}
	logger := testhelpers.NewTestLogger()
	ch := make(chan stats)

	go func() {
		defer close(ch)

		handleStats(t.Context(), containerID, &dockerClient, networkCountConfig, logger, ch)
	}()

	go func() {
		defer pw.Close()

		scanner := bufio.NewScanner(bytes.NewReader(statsWithRestartJSON))
		for scanner.Scan() {
			_, err := pw.Write(scanner.Bytes())
			require.NoError(t, err)
		}
	}()

	// before container restart:
	var lastStats stats
	for range 31 {
		lastStats = <-ch
	}

	assert.Equal(t, 5.008083989501312, lastStats.cpuPercent)
	assert.Equal(t, uint64(24068096), lastStats.memoryUsageBytes)
	assert.Equal(t, 1199, lastStats.rxRate)
	assert.Equal(t, 1200, lastStats.txRate)

	// restart triggers zero value stats:
	assert.Equal(t, stats{}, <-ch)

	// after container restart:
	for stats := range ch {
		lastStats = stats
	}

	assert.Equal(t, 1.887690322580645, lastStats.cpuPercent)
	assert.Equal(t, uint64(12496896), lastStats.memoryUsageBytes)
	assert.Equal(t, 1215, lastStats.rxRate)
	assert.Equal(t, 1254, lastStats.txRate)
}
