package main

import (
	"cmp"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"

	"git.netflux.io/rob/octoplex/internal/client"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/server"
	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/xdg"
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

const (
	defaultListenAddr = "127.0.0.1:50051"
	defaultHostname   = "localhost"
	defaultStreamKey  = "live"
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

// CLI flags that must be overridden in "run" mode.
// Workaround for urfave/cli not really supporting overriding flags
// programmatically.
var (
	serverListenAddr    string
	serverHostname      string
	clientHost          string
	clientTLSSkipVerify bool
)

func main() {
	app := &cli.App{
		Name:  "octoplex",
		Usage: "Live video restreamer for Docker",
		Commands: []*cli.Command{
			{
				Name:        "run",
				Usage:       "Launch both server and client in a single process",
				Description: "Launch both server and client in a single process. This is useful for testing, debugging or streaming from the same machine that runs Docker (e.g. a laptop).",
				Flags:       serverFlags(true),
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
						Flags:       serverFlags(false),
						Action: func(c *cli.Context) error {
							cfg, err := parseConfig(c)
							if err != nil {
								return fmt.Errorf("parse config: %w", err)
							}

							logger, err := buildServerLogger(cfg, true)
							if err != nil {
								return fmt.Errorf("build logger: %w", err)
							}

							return runServer(c.Context, c, cfg, serverConfig{stderrAvailable: true, handleSigInt: true, waitForClient: false}, logger.With("component", "server"))
						},
					},
					{
						Name:        "stop",
						Usage:       "Stop the server",
						Description: "Stop all containers and networks created by Octoplex, and exit.",
						Action: func(c *cli.Context) error {
							// Delegate to start command:
							return c.App.Command("server").Subcommands[0].Action(c)
						},
					},
				},
			},
			{
				Name:  "client",
				Usage: "Manage the client",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "host",
						Usage:       "The remote Octoplex server to connect to",
						Value:       client.DefaultServerAddr,
						Destination: &clientHost,
					},
					&cli.BoolFlag{
						Name:        "tls-skip-verify",
						Usage:       "Skip TLS verification (insecure)",
						DefaultText: "false",
						Destination: &clientTLSSkipVerify,
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
							logger, err := buildClientLogger(c)
							if err != nil {
								return fmt.Errorf("build logger: %w", err)
							}

							return runClient(c.Context, c, logger)
						},
					},
				},
				Action: func(c *cli.Context) error {
					logger, err := buildClientLogger(c)
					if err != nil {
						return fmt.Errorf("build logger: %w", err)
					}

					return runClient(c.Context, c, logger)
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

		var authErr x509.UnknownAuthorityError
		if errors.As(err, &authErr) {
			os.Stderr.WriteString("Hint: Run with --tls-skip-verify to ignore.\n")
		}

		os.Exit(1)
	}
}

func serverFlags(clientAndServerMode bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "listen-addr",
			Usage:       "The address to listen on",
			DefaultText: defaultListenAddr,
			Category:    "Server",
			Aliases:     []string{"a"},
			EnvVars:     []string{"OCTO_LISTEN_ADDR"},
			Destination: &serverListenAddr,
			Hidden:      clientAndServerMode,
		},
		&cli.StringFlag{
			Name:        "hostname",
			Usage:       "The public DNS name of the server",
			DefaultText: defaultHostname,
			Category:    "Server",
			Aliases:     []string{"H"},
			EnvVars:     []string{"OCTO_HOSTNAME"},
			Destination: &serverHostname,
			Hidden:      clientAndServerMode,
		},
		&cli.BoolFlag{
			Name:     "in-docker",
			Usage:    "Configure the server to run inside Docker",
			Category: "Server",
			EnvVars:  []string{"OCTO_DOCKER"},
			Hidden:   clientAndServerMode,
		},
		&cli.StringFlag{
			Name:      "tls-cert",
			Usage:     "Path to a TLS certificate",
			Category:  "Server",
			TakesFile: true,
			EnvVars:   []string{"OCTO_TLS_CERT"},
		},
		&cli.StringFlag{
			Name:      "tls-key",
			Usage:     "Path to a TLS key",
			Category:  "Server",
			TakesFile: true,
			EnvVars:   []string{"OCTO_TLS_KEY"},
		},
		&cli.BoolFlag{
			Name:     "log-to-file",
			Usage:    "Write logs to file instead of stderr",
			Category: "Logs",
			EnvVars:  []string{"OCTO_LOG_TO_FILE"},
		},
		&cli.StringFlag{
			Name:      "log-file",
			Usage:     "Path to the log file (implies log-to-file)",
			Category:  "Logs",
			TakesFile: true,
			EnvVars:   []string{"OCTO_LOG_FILE"},
		},
		&cli.BoolFlag{
			Name:     "debug", // TODO: replace with --log-level
			Usage:    "Enable debug logging",
			Category: "Logs",
			EnvVars:  []string{"OCTO_DEBUG"},
		},
		&cli.StringFlag{
			Name:        "stream-key",
			Usage:       "Stream key for RTMP sources",
			Category:    "Sources",
			DefaultText: defaultStreamKey,
			EnvVars:     []string{"OCTO_STREAM_KEY"},
		},
		&cli.BoolFlag{
			Name:        "rtmp-enabled",
			Usage:       "Enable the RTMP source",
			Category:    "Sources",
			DefaultText: "true",
		},
		&cli.StringFlag{
			Name:        "rtmp-listen-addr",
			Usage:       "The address to listen on for RTMP",
			Category:    "Sources",
			DefaultText: "127.0.0.1:1935",
		},
		&cli.BoolFlag{
			Name:        "rtmps-enabled",
			Usage:       "Enable the RTMPS source",
			Category:    "Sources",
			DefaultText: "false",
		},
		&cli.StringFlag{
			Name:        "rtmps-listen-addr",
			Usage:       "The address to listen on for RTMPS",
			Category:    "Sources",
			DefaultText: "127.0.0.1:1936",
		},
	}
}

// runClient runs the client.
func runClient(ctx context.Context, _ *cli.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger.Info("Starting client", "version", cmp.Or(version, "devel"), "commit", cmp.Or(commit, "unknown"), "date", cmp.Or(date, "unknown"), "go_version", runtime.Version())

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("unable to read build info")
	}

	app := client.New(client.NewParams{
		ServerAddr:         clientHost,
		InsecureSkipVerify: clientTLSSkipVerify,
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

// serverConfig holds additional configuration for launching the server.
type serverConfig struct {
	listenerFunc    server.ListenerFunc // override config
	stderrAvailable bool
	handleSigInt    bool
	waitForClient   bool
}

// runServer runs the server.
func runServer(ctx context.Context, c *cli.Context, cfg config.Config, serverCfg serverConfig, logger *slog.Logger) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

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

	store, err := store.New(store.DefaultPath)
	if err != nil {
		return fmt.Errorf("new store: %w", err)
	}

	app, err := server.New(server.Params{
		Config:        cfg,
		Store:         store,
		DockerClient:  dockerClient,
		ListenerFunc:  serverCfg.listenerFunc,
		WaitForClient: serverCfg.waitForClient,
		Logger:        logger,
	})
	if err != nil {
		return fmt.Errorf("new server: %w", err)
	}

	if c.Command.Name == "stop" {
		return app.Stop(ctx)
	}

	logger.Info("Starting server", "version", cmp.Or(version, "devel"), "commit", cmp.Or(commit, "unknown"), "date", cmp.Or(date, "unknown"), "go_version", runtime.Version())

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

// runClientAndServer runs client and server in the same process.
func runClientAndServer(c *cli.Context) error {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Override CLI flags:
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", lis.Addr().(*net.TCPAddr).Port)
	serverHostname = "localhost"
	clientHost = fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)
	clientTLSSkipVerify = true

	// must be built after overriding flags:
	cfg, err := parseConfig(c)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// must be built after overriding flags:
	logger, err := buildServerLogger(cfg, false)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	errNoErr := errors.New("no error")
	g, ctx := errgroup.WithContext(c.Context)

	g.Go(func() error {
		if err := runServer(
			ctx,
			c,
			cfg,
			serverConfig{listenerFunc: server.WithListener(lis), stderrAvailable: false, handleSigInt: false, waitForClient: true},
			logger.With("component", "server"),
		); err != nil {
			return err
		}

		return errNoErr
	})

	g.Go(func() error {
		if err := runClient(ctx, c, logger.With("component", "client")); err != nil {
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

// printVersion prints the version of the application to stderr.
func printVersion() error {
	fmt.Fprintf(os.Stderr, "%s version %s\n", domain.AppName, cmp.Or(version, "0.0.0-dev"))
	return nil
}

func parseConfig(c *cli.Context) (config.Config, error) {
	logToFileEnabled := c.Bool("log-to-file")
	logFile := c.String("log-file")
	if logToFileEnabled && logFile == "" {
		appStateDir, err := xdg.CreateAppStateDir()
		if err != nil {
			return config.Config{}, fmt.Errorf("create app state dir: %w", err)
		}
		logFile = filepath.Join(appStateDir, "octoplex.log")
	}

	cfg := config.Config{
		ListenAddr: cmp.Or(serverListenAddr, defaultListenAddr),
		Host:       cmp.Or(serverHostname, defaultHostname),
		InDocker:   c.Bool("in-docker"),
		Debug:      c.Bool("debug"),
		LogFile: config.LogFile{
			Enabled: logToFileEnabled,
			Path:    logFile,
		},
	}

	tlsCertSet := c.IsSet("tls-cert")
	tlsKeySet := c.IsSet("tls-key")
	if tlsCertSet != tlsKeySet {
		return config.Config{}, fmt.Errorf("both --tls-cert and --tls-key must be set")
	}
	if tlsCertSet {
		cfg.TLS = &config.TLS{
			CertPath: c.String("tls-cert"),
			KeyPath:  c.String("tls-key"),
		}
	}

	cfg.Sources.MediaServer.StreamKey = cmp.Or(c.String("stream-key"), defaultStreamKey)

	rtmpEnabled := true
	if c.IsSet("rtmp-enabled") {
		rtmpEnabled = c.Bool("rtmp-enabled")
	}
	cfg.Sources.MediaServer.RTMP.Enabled = rtmpEnabled
	if rtmpEnabled {
		if err := parseRTMPConfig(&cfg.Sources.MediaServer.RTMP, c, "rtmp-listen-addr"); err != nil {
			return config.Config{}, fmt.Errorf("parse RTMP: %w", err)
		}
	}

	rtmpsEnabled := c.Bool("rtmps-enabled")
	cfg.Sources.MediaServer.RTMPS.Enabled = rtmpsEnabled
	if rtmpsEnabled {
		if err := parseRTMPConfig(&cfg.Sources.MediaServer.RTMPS, c, "rtmps-listen-addr"); err != nil {
			return config.Config{}, fmt.Errorf("parse RTMP: %w", err)
		}
	}

	return cfg, nil
}

func parseRTMPConfig(cfg *config.RTMPSource, c *cli.Context, arg string) error {
	if !c.IsSet(arg) {
		return nil
	}

	host, portStr, err := net.SplitHostPort(c.String(arg))
	if err != nil {
		return fmt.Errorf("split host port: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("atoi: %w", err)
	}
	cfg.IP = host
	cfg.Port = port

	return nil
}

func buildServerLogger(cfg config.Config, stderrAvailable bool) (*slog.Logger, error) {
	var w io.Writer
	if stderrAvailable {
		w = os.Stdout
	} else if !cfg.LogFile.Enabled {
		w = io.Discard
	} else {
		var err error
		w, err = os.OpenFile(cfg.LogFile.GetPath(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("error opening log file: %w", err)
		}
	}

	var handlerOpts slog.HandlerOptions
	if cfg.Debug {
		handlerOpts.Level = slog.LevelDebug
	}

	return slog.New(slog.NewTextHandler(w, &handlerOpts)), nil
}

func buildClientLogger(c *cli.Context) (*slog.Logger, error) {
	logger := slog.New(slog.DiscardHandler)

	if logfile := c.String("log-file"); logfile != "" {
		fptr, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		logger = slog.New(slog.NewTextHandler(fptr, nil))
	}

	return logger, nil
}
