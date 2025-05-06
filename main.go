package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"git.netflux.io/rob/octoplex/internal/client"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/server"
	dockerclient "github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
	"golang.design/x/clipboard"
	"golang.org/x/sync/errgroup"
)

var (
	// version is the version of the application.
	version string
	// commit is the commit hash of the application.
	commit string
	// date is the date of the build.
	date string
)

// errInterrupt is an error type that indicates an interrupt signal was
// received.
type errInterrupt struct{}

// Error implements the error interface.
func (e errInterrupt) Error() string {
	return "interrupt signal received"
}

// ExitCode implements the ExitCoder interface.
func (e errInterrupt) ExitCode() int {
	return 130
}

func main() {
	app := &cli.App{
		Name:  "Octoplex",
		Usage: "Octoplex is a live video restreamer for Docker.",
		Commands: []*cli.Command{
			{
				Name:  "client",
				Usage: "Run the client",
				Action: func(c *cli.Context) error {
					return runClient(c.Context, c)
				},
			},
			{
				Name:  "server",
				Usage: "Run the server",
				Action: func(c *cli.Context) error {
					return runServer(c.Context, c, serverConfig{
						stderrAvailable: true,
						handleSigInt:    true,
						waitForClient:   false,
					})
				},
			},
			{
				Name:  "run",
				Usage: "Run server and client together (testing)",
				Action: func(c *cli.Context) error {
					return runClientAndServer(c)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runClient(ctx context.Context, _ *cli.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// TODO: logger from config
	fptr, err := os.OpenFile("octoplex.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(fptr, nil))
	logger.Info("Starting client", "version", cmp.Or(version, "devel"), "commit", cmp.Or(commit, "unknown"), "date", cmp.Or(date, "unknown"), "go_version", runtime.Version())

	var clipboardAvailable bool
	if err = clipboard.Init(); err != nil {
		logger.Warn("Clipboard not available", "err", err)
	} else {
		clipboardAvailable = true
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("read build info: %w", err)
	}

	app := client.New(client.NewParams{
		ClipboardAvailable: clipboardAvailable,
		BuildInfo: domain.BuildInfo{
			GoVersion: buildInfo.GoVersion,
			Version:   version,
			Commit:    commit,
			Date:      date,
		},
		Logger: logger,
	})
	if err := app.Run(ctx); err != nil {
		return fmt.Errorf("run app: %w", err)
	}

	return nil
}

type serverConfig struct {
	stderrAvailable bool
	handleSigInt    bool
	waitForClient   bool
}

func runServer(ctx context.Context, _ *cli.Context, serverCfg serverConfig) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	configService, err := config.NewDefaultService()
	if err != nil {
		return fmt.Errorf("build config service: %w", err)
	}

	cfg, err := configService.ReadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("read or create config: %w", err)
	}

	// TODO: improve logger API
	// Currently it's a bit complicated because we can only use stdout - the
	// preferred destination - if the client is not running. Otherwise we
	// fallback to the legacy configuration but this should be bought more
	// in-line with the client/server split.
	var w io.Writer
	if serverCfg.stderrAvailable {
		w = os.Stdout
	} else if !cfg.LogFile.Enabled {
		w = io.Discard
	} else {
		w, err = os.OpenFile(cfg.LogFile.GetPath(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("error opening log file: %w", err)
		}
	}

	var handlerOpts slog.HandlerOptions
	if os.Getenv("OCTO_DEBUG") != "" {
		handlerOpts.Level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(w, &handlerOpts))

	if serverCfg.handleSigInt {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-ch
			logger.Info("Received interrupt signal, exiting")
			signal.Stop(ch)
			cancel(errInterrupt{})
		}()
	}

	dockerClient, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	app := server.New(server.Params{
		ConfigService:  configService,
		DockerClient:   dockerClient,
		ConfigFilePath: configService.Path(),
		WaitForClient:  serverCfg.waitForClient,
		Logger:         logger,
	})

	logger.Info(
		"Starting server",
		"version",
		cmp.Or(version, "devel"),
		"commit",
		cmp.Or(commit, "unknown"),
		"date",
		cmp.Or(date, "unknown"),
		"go_version",
		runtime.Version(),
	)

	if err := app.Run(ctx); err != nil {
		if errors.Is(err, context.Canceled) && errors.Is(context.Cause(ctx), errInterrupt{}) {
			return context.Cause(ctx)
		}
		return err
	}

	return nil
}

func runClientAndServer(c *cli.Context) error {
	errNoErr := errors.New("no error")

	g, ctx := errgroup.WithContext(c.Context)

	g.Go(func() error {
		if err := runClient(ctx, c); err != nil {
			return err
		}

		return errNoErr
	})

	g.Go(func() error {
		if err := runServer(ctx, c, serverConfig{
			stderrAvailable: false,
			handleSigInt:    false,
			waitForClient:   true,
		}); err != nil {
			return err
		}

		return errNoErr
	})

	if err := g.Wait(); err == errNoErr {
		return nil
	} else {
		return err
	}
}
