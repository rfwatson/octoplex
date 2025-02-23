package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"git.netflux.io/rob/termstream/app"
	"git.netflux.io/rob/termstream/config"
	dockerclient "github.com/docker/docker/client"
	"golang.design/x/clipboard"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, config.FromFile()); err != nil {
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

func run(ctx context.Context, cfgReader io.Reader) error {
	cfg, err := config.Load(cfgReader)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logFile, err := os.OpenFile(cfg.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(logFile, nil))

	var clipboardAvailable bool
	if err = clipboard.Init(); err != nil {
		logger.Warn("Clipboard not available", "err", err)
	} else {
		clipboardAvailable = true
	}

	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	return app.Run(
		ctx,
		cfg,
		dockerClient,
		clipboardAvailable,
		logger,
	)
}
