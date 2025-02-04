package container

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/docker/docker/api/types/container"
)

func handleStats(
	ctx context.Context,
	containerID string,
	apiClient DockerClient,
	networkCountConfig NetworkCountConfig,
	logger *slog.Logger,
	ch chan<- stats,
) {
	networkNameRx := cmp.Or(networkCountConfig.Rx, "eth0")
	networkNameTx := cmp.Or(networkCountConfig.Tx, "eth0")

	statsReader, err := apiClient.ContainerStats(ctx, containerID, true)
	if err != nil {
		// TODO: error handling?
		logger.Error("Error getting container stats", "err", err, "id", shortID(containerID))
		return
	}
	defer statsReader.Body.Close()

	var (
		processedAny  bool
		lastNetworkRx uint64
		lastNetworkTx uint64
	)

	getAvgRxRate := rolling(10)
	getAvgTxRate := rolling(10)

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

		// https://stackoverflow.com/a/30292327/62871
		cpuDelta := float64(statsResp.CPUStats.CPUUsage.TotalUsage - statsResp.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(statsResp.CPUStats.SystemUsage - statsResp.PreCPUStats.SystemUsage)

		var avgRxRate, avgTxRate float64
		if processedAny {
			secondsSinceLastReceived := statsResp.Read.Sub(statsResp.PreRead).Seconds()
			diffRxBytes := (statsResp.Networks[networkNameRx].RxBytes - lastNetworkRx)
			diffTxBytes := (statsResp.Networks[networkNameTx].TxBytes - lastNetworkTx)
			rxRate := float64(diffRxBytes) / secondsSinceLastReceived / 1000.0 * 8
			txRate := float64(diffTxBytes) / secondsSinceLastReceived / 1000.0 * 8
			avgRxRate = getAvgRxRate(rxRate)
			avgTxRate = getAvgTxRate(txRate)
		}

		lastNetworkRx = statsResp.Networks[networkNameRx].RxBytes
		lastNetworkTx = statsResp.Networks[networkNameTx].TxBytes
		processedAny = true

		ch <- stats{
			cpuPercent:       (cpuDelta / systemDelta) * float64(statsResp.CPUStats.OnlineCPUs) * 100,
			memoryUsageBytes: statsResp.MemoryStats.Usage,
			rxRate:           int(avgRxRate),
			txRate:           int(avgTxRate),
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
