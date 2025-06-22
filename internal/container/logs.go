package container

import (
	"bufio"
	"context"
	"log/slog"

	typescontainer "github.com/docker/docker/api/types/container"
)

func getLogs(
	ctx context.Context,
	containerID string,
	apiClient DockerClient,
	cfg LogConfig,
	ch chan<- []byte,
	logger *slog.Logger,
) {
	logsC, err := apiClient.ContainerLogs(
		ctx,
		containerID,
		typescontainer.LogsOptions{
			ShowStdout: cfg.Stdout,
			ShowStderr: cfg.Stderr,
			Follow:     true,
		},
	)
	if err != nil {
		logger.Error("Error getting container logs", "err", err, "id", shortID(containerID))
		return
	}
	defer logsC.Close() //nolint:errcheck

	scanner := bufio.NewScanner(logsC)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			// Docker logs are prefixed with an 8 byte prefix.
			// See client.ContainerLogs for more details.
			// We could use
			// [StdCopy](https://pkg.go.dev/github.com/docker/docker/pkg/stdcopy#StdCopy)
			// but for our purposes it's enough to just slice it off.
			const prefixLen = 8
			line := scanner.Bytes()
			if len(line) <= prefixLen {
				continue
			}
			ch <- line[prefixLen:]
		}
	}
}
