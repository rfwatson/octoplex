package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"git.netflux.io/rob/octoplex/internal/client"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/server"
	"github.com/atotto/clipboard"
	dockerclient "github.com/docker/docker/client"
	"github.com/urfave/cli/v2"
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
		Name:  "octoplex",
		Usage: "Live video restreamer for Docker",
		Commands: []*cli.Command{
			{
				Name:        "run",
				Usage:       "Launch both server and client in a single process",
				Description: "Launch both server and client in a single process. This is useful for testing, debugging or running for a single user.",
				Action: func(c *cli.Context) error {
					return runClientAndServer(c)
				},
			},
			{
				Name:  "server",
				Usage: "Manage the standalone Octoplex server",
				Action: func(c *cli.Context) error {
					return c.App.Command("server").Subcommands[0].Action(c)
				},
				Subcommands: []*cli.Command{
					{
						Name:        "start",
						Usage:       "Start the server",
						Description: "Start the standalone server, without a CLI client attached.",
						Action: func(c *cli.Context) error {
							return runServer(c.Context, c, serverConfig{
								stderrAvailable: true,
								handleSigInt:    true,
								waitForClient:   false,
							})
						},
					},
					{
						Name:        "stop",
						Usage:       "Stop the server",
						Description: "Stop all containers and networks created by Octoplex, and exit.",
						Action: func(c *cli.Context) error {
							return runServer(c.Context, c, serverConfig{
								stderrAvailable: true,
								handleSigInt:    false,
								waitForClient:   false,
							})
						},
					},
					{
						Name:  "print-config",
						Usage: "Print the config file path",
						Action: func(*cli.Context) error {
							return printConfig()
						},
					},
					{
						Name:  "edit-config",
						Usage: "Edit the config file",
						Action: func(*cli.Context) error {
							return editConfig()
						},
					},
				},
			},
			{
				Name:  "client",
				Usage: "Manage the client",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "host",
						Usage: "The remote Octoplex server to connect to",
						Value: client.DefaultServerAddr,
					},
					&cli.BoolFlag{
						Name:        "tls-skip-verify",
						Usage:       "Skip TLS verification (insecure)",
						DefaultText: "false",
					},
					&cli.StringFlag{
						Name:      "log-file",
						Usage:     "Path to the log file",
						TakesFile: true,
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:        "start",
						Usage:       "Start the TUI client",
						Description: "Start the terminal user interface, connecting to a remote server.",
						Action: func(c *cli.Context) error {
							return runClient(c.Context, c, clientConfig{})
						},
					},
				},
				Action: func(c *cli.Context) error {
					return runClient(c.Context, c, clientConfig{})
				},
			},
			{
				Name:  "version",
				Usage: "Display the currrent version",
				Action: func(*cli.Context) error {
					return printVersion()
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		if strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
			os.Stderr.WriteString("Hint: Run with --tls-skip-verify to ignore.\n")
		}

		os.Exit(1)
	}
}

// runClient runs the client, using the provided options (but
// overriding the host address if it's non-zero).
func runClient(ctx context.Context, c *cli.Context, cfg clientConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := slog.New(slog.DiscardHandler)
	if logfile := c.String("log-file"); logfile != "" {
		fptr, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		logger = slog.New(slog.NewTextHandler(fptr, nil))
	}

	logger.Info("Starting client", "version", cmp.Or(version, "devel"), "commit", cmp.Or(commit, "unknown"), "date", cmp.Or(date, "unknown"), "go_version", runtime.Version())

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("unable to read build info")
	}

	app := client.New(client.NewParams{
		ServerAddr:         cmp.Or(cfg.remoteHost, c.String("host")),
		InsecureSkipVerify: cmp.Or(cfg.insecureSkipVerify, c.Bool("tls-skip-verify")),
		ClipboardAvailable: !clipboard.Unsupported,
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
	listenerFunc    server.ListenerFunc // to override config
	stderrAvailable bool
	handleSigInt    bool
	waitForClient   bool
}

type clientConfig struct {
	remoteHost         string
	insecureSkipVerify bool
}

func runServer(ctx context.Context, c *cli.Context, serverCfg serverConfig) error {
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

	app, err := server.New(server.Params{
		ConfigService:  configService,
		DockerClient:   dockerClient,
		ListenerFunc:   serverCfg.listenerFunc,
		ConfigFilePath: configService.Path(),
		WaitForClient:  serverCfg.waitForClient,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("new server: %w", err)
	}

	if c.Command.Name == "stop" {
		return app.Stop(ctx)
	}

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
		if errors.Is(err, server.ErrOtherInstanceDetected) {
			msg := "Another instance of the server may be running.\n" +
				"To stop the server, run `octoplex server stop`."
			return cli.Exit(msg, 1)
		}
		return err
	}

	return nil
}

func runClientAndServer(c *cli.Context) error {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	errNoErr := errors.New("no error")
	g, ctx := errgroup.WithContext(c.Context)

	g.Go(func() error {
		if err := runServer(ctx, c, serverConfig{
			listenerFunc:    server.WithListener(lis),
			stderrAvailable: false,
			handleSigInt:    false,
			waitForClient:   true,
		}); err != nil {
			return err
		}

		return errNoErr
	})

	g.Go(func() error {
		if err := runClient(ctx, c, clientConfig{
			remoteHost:         lis.Addr().String(),
			insecureSkipVerify: true,
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

// printConfig prints the path to the config file to stderr.
func printConfig() error {
	configService, err := config.NewDefaultService()
	if err != nil {
		return fmt.Errorf("build config service: %w", err)
	}

	fmt.Fprintln(os.Stderr, configService.Path())
	return nil
}

// editConfig opens the config file in the user's editor.
func editConfig() error {
	configService, err := config.NewDefaultService()
	if err != nil {
		return fmt.Errorf("build config service: %w", err)
	}

	if _, err = configService.ReadOrCreateConfig(); err != nil {
		return fmt.Errorf("read or create config: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	binary, err := exec.LookPath(editor)
	if err != nil {
		return fmt.Errorf("look path: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Editing config file: %s\n", configService.Path())

	if err := syscall.Exec(binary, []string{"--", configService.Path()}, os.Environ()); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	return nil
}

// printVersion prints the version of the application to stderr.
func printVersion() error {
	fmt.Fprintf(os.Stderr, "%s version %s\n", domain.AppName, cmp.Or(version, "0.0.0-dev"))
	return nil
}
