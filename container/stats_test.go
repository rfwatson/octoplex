package container

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"io"
	"testing"

	"git.netflux.io/rob/termstream/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/stats1.json
var statsJSON []byte

func TestHandleStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pr, pw := io.Pipe()
	containerID := "b905f51b47242090ae504c184c7bc84d6274511ef763c1847039dcaa00a3ad27"
	dockerClient := testhelpers.MockDockerClient{ContainerStatsResponse: pr}
	networkCountConfig := NetworkCountConfig{Rx: "eth0", Tx: "eth1"}
	logger := testhelpers.NewTestLogger()
	ch := make(chan stats)

	go func() {
		defer close(ch)

		handleStats(ctx, containerID, &dockerClient, networkCountConfig, logger, ch)
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
