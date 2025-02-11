package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"git.netflux.io/rob/termstream/app"
	"git.netflux.io/rob/termstream/config"
	dockerclient "github.com/docker/docker/client"
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

	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	return app.Run(ctx, cfg, dockerClient)
}
