package container

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/container"
)

func handleStats(
	ctx context.Context,
	containerID string,
	apiClient DockerClient,
	logger *slog.Logger,
	ch chan<- stats,
) {
	statsReader, err := apiClient.ContainerStats(ctx, containerID, true)
	if err != nil {
		// TODO: error handling?
		logger.Error("Error getting container stats", "err", err, "id", shortID(containerID))
		return
	}
	defer statsReader.Body.Close() //nolint:errcheck

	var (
		lastRxBytes  uint64
		lastTxBytes  uint64
		getAvgRxRate func(float64) float64
		getAvgTxRate func(float64) float64
		rxSince      time.Time
	)

	reset := func() {
		lastRxBytes = 0
		lastTxBytes = 0
		getAvgRxRate = rolling(10)
		getAvgTxRate = rolling(10)
		rxSince = time.Time{}
	}
	reset()

	buf := make([]byte, 4_096)
	for {
		n, err := statsReader.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				break
			}
			logger.Error("Error reading stats", "err", err, "id", shortID(containerID))
			break
		}

		var statsResp container.StatsResponse
		if err = json.Unmarshal(buf[:n], &statsResp); err != nil {
			logger.Error("Error unmarshalling stats", "err", err, "id", shortID(containerID))
			break
		}

		// Get the first network, which is the only one.
		var networkName string
		for name := range statsResp.Networks {
			networkName = name
			break
		}
		if networkName == "" {
			logger.Warn("No network stats found", "id", shortID(containerID))
			continue
		}

		rxBytes := statsResp.Networks[networkName].RxBytes
		txBytes := statsResp.Networks[networkName].TxBytes

		if statsResp.PreRead.IsZero() || rxBytes < lastRxBytes || txBytes < lastTxBytes {
			// Container restarted
			reset()
			lastRxBytes = rxBytes
			lastTxBytes = txBytes
			ch <- stats{}
			continue
		}

		// https://stackoverflow.com/a/30292327/62871
		var cpuDelta, systemDelta float64
		cpuDelta = float64(statsResp.CPUStats.CPUUsage.TotalUsage - statsResp.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta = float64(statsResp.CPUStats.SystemUsage - statsResp.PreCPUStats.SystemUsage)

		secondsSinceLastReceived := statsResp.Read.Sub(statsResp.PreRead).Seconds()
		diffRxBytes := rxBytes - lastRxBytes
		diffTxBytes := txBytes - lastTxBytes
		rxRate := float64(diffRxBytes) / secondsSinceLastReceived / 1000.0 * 8
		txRate := float64(diffTxBytes) / secondsSinceLastReceived / 1000.0 * 8
		avgRxRate := getAvgRxRate(rxRate)
		avgTxRate := getAvgTxRate(txRate)

		if diffRxBytes > 0 && rxSince.IsZero() {
			rxSince = statsResp.PreRead
		}

		lastRxBytes = rxBytes
		lastTxBytes = txBytes

		ch <- stats{
			cpuPercent:       (cpuDelta / systemDelta) * float64(statsResp.CPUStats.OnlineCPUs) * 100,
			memoryUsageBytes: statsResp.MemoryStats.Usage,
			rxRate:           int(avgRxRate),
			txRate:           int(avgTxRate),
			rxSince:          rxSince,
		}
	}
}

// https://stackoverflow.com/a/12539781/62871
func rolling(n int) func(float64) float64 {
	bins := make([]float64, n)
	var avg float64
	var i int

	return func(x float64) float64 {
		avg += (x - bins[i]) / float64(n)
		bins[i] = x
		i = (i + 1) % n
		return avg
	}
}
