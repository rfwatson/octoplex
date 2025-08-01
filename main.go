package main

import (
	"cmp"
	"context"
	"crypto/x509"
	"encoding/json"
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
	"text/tabwriter"

	"git.netflux.io/rob/octoplex/internal/client"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/mediaserver"
	"git.netflux.io/rob/octoplex/internal/optional"
	"git.netflux.io/rob/octoplex/internal/replicator"
	"git.netflux.io/rob/octoplex/internal/server"
	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/xdg"
	"github.com/atotto/clipboard"
	dockerclient "github.com/docker/docker/client"
	"github.com/urfave/cli/v3"
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
	defaultListenAddr    = "127.0.0.1:8080"
	defaultListenAddrTLS = "127.0.0.1:8443"
	defaultServerURL     = "http://localhost:8080"
	defaultStreamKey     = "live"
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
	serverURL           string
	serverAuthMode      string
	webEnabled          bool
	insecureAllowNoAuth bool
	clientHost          string
	clientTLSSkipVerify bool
)

//go:generate mise run build_assets
//go:generate buf generate
//go:generate go tool mockery
func main() {
	if err := run(context.Background(), os.Stdout, os.Stderr, os.Args); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	app := &cli.Command{
		Name:  "octoplex",
		Usage: "Live video restreamer for Docker",
		Commands: []*cli.Command{
			{
				Name:        "run",
				Usage:       "Launch both server and client in a single process",
				Description: "Launch both server and client in a single process. This is useful for testing, debugging or streaming from the same machine that runs Docker (e.g. a laptop).",
				Flags:       serverFlags(true),
				Action: func(ctx context.Context, c *cli.Command) error {
					return runClientAndServer(ctx, c)
				},
			},
			{
				Name:  "server",
				Usage: "Manage the standalone Octoplex server",
				Action: func(ctx context.Context, c *cli.Command) error {
					return c.Commands[0].Action(ctx, c)
				},
				Commands: []*cli.Command{
					{
						Name:        "start",
						Usage:       "Start the server",
						Description: "Start the standalone server, without a CLI client attached.",
						Flags:       serverFlags(false),
						Action: func(ctx context.Context, c *cli.Command) error {
							cfg, err := parseConfig(c)
							if err != nil {
								return fmt.Errorf("parse config: %w", err)
							}

							logger, err := buildServerLogger(cfg, stderr, true)
							if err != nil {
								return fmt.Errorf("build logger: %w", err)
							}

							return runServer(ctx, c, cfg, serverConfig{stderrAvailable: true, handleSigInt: true, waitForClient: false}, logger.With("component", "server"))
						},
					},
					{
						Name:        "stop",
						Usage:       "Stop the server",
						Description: "Stop all containers and networks created by Octoplex, and exit.",
						Action: func(ctx context.Context, c *cli.Command) error {
							// Delegate to start command:
							return c.Root().Command("server").Command("start").Action(ctx, c)
						},
					},
					{
						Name:        "credentials",
						Usage:       "Manage server credentials",
						Description: "Manage server credentials.",
						Commands: []*cli.Command{
							{
								Name:        "reset",
								Usage:       "Reset server credentials, and print them to stdout.",
								Description: "Reset the server credentials, and print them to stdout.",
								Flags:       []cli.Flag{flagDataDir()},
								Action: func(ctx context.Context, c *cli.Command) error {
									cfg, err := parseConfig(c)
									if err != nil {
										return fmt.Errorf("parse config: %w", err)
									}

									logger, err := buildServerLogger(cfg, stderr, true)
									if err != nil {
										return fmt.Errorf("build logger: %w", err)
									}

									apiToken, adminPassword, err := server.ResetCredentials(cfg, store.NewTokenStore(cfg.DataDir, logger))
									if err != nil {
										return fmt.Errorf("reset credentials: %w", err)
									}

									output := struct {
										APIToken      string `json:"api_token"`
										AdminPassword string `json:"admin_password,omitzero"`
									}{APIToken: apiToken, AdminPassword: adminPassword}

									bytes, err := json.MarshalIndent(output, "", "  ")
									if err != nil {
										return fmt.Errorf("marshal output: %w", err)
									}

									if _, err := stdout.Write(bytes); err != nil {
										return fmt.Errorf("write: %w", err)
									}

									return nil
								},
							},
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
						Aliases:     []string{"H"},
						Destination: &clientHost,
					},
					&cli.BoolFlag{
						Name:        "tls-skip-verify",
						Usage:       "Skip TLS verification (insecure)",
						Aliases:     []string{"k"},
						DefaultText: "false",
						Destination: &clientTLSSkipVerify,
					},
					&cli.StringFlag{
						Name:    "api-token",
						Usage:   "API token for authentication with the server",
						Aliases: []string{"t"},
					},
					&cli.StringFlag{
						Name:      "log-file",
						Usage:     "Path to the log file",
						TakesFile: true,
					},
					&cli.StringFlag{
						Name:        "log-level",
						Usage:       "Set the logging level",
						DefaultText: "info",
					},
				},
				Commands: []*cli.Command{
					{
						Name:        "start",
						Usage:       "Start the TUI client",
						Description: "Start the terminal user interface, connecting to a remote server.",
						Action: func(ctx context.Context, c *cli.Command) error {
							logger, err := buildClientLogger(c)
							if err != nil {
								return fmt.Errorf("build logger: %w", err)
							}

							return runClient(ctx, c, logger)
						},
					},
					{
						Name:    "destination",
						Usage:   "Manage destinations",
						Aliases: []string{"dest", "destinations"},
						Commands: []*cli.Command{
							{
								Name:        "list",
								Usage:       "List existing destinations",
								Description: "List existing destinations on the server.",
								Action: func(ctx context.Context, c *cli.Command) error {
									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									if destinations, err := client.ListDestinations(ctx); err != nil {
										return fmt.Errorf("list destinations: %w", err)
									} else {
										w := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
										if _, err := fmt.Fprintf(w, "ID\tName\tURL\tStatus\n"); err != nil {
											return handleStdoutError(err)
										}
										for _, dest := range destinations {
											if _, err := fmt.Fprintf(
												w,
												"%s\t%s\t%s\t%s\n",
												dest.ID,
												dest.Name,
												dest.URL,
												dest.Status.String(),
											); err != nil {
												return handleStdoutError(err)
											}
										}
										if err := w.Flush(); err != nil {
											return handleStdoutError(err)
										}

										return nil
									}
								},
							},
							{
								Name:        "add",
								Usage:       "Add a new destination",
								Description: "Add a new destination to the server.",
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:     "name",
										Usage:    "Name of the destination",
										Aliases:  []string{"n"},
										Required: true,
									},
									&cli.StringFlag{
										Name:     "url",
										Usage:    "RTMP URL of the destination",
										Aliases:  []string{"u"},
										Required: true,
									},
								},
								Action: func(ctx context.Context, c *cli.Command) error {
									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									id, err := client.AddDestination(ctx, c.String("name"), c.String("url"))
									if err != nil {
										return fmt.Errorf("add destination: %w", err)
									}

									if _, err := stdout.Write([]byte(id + "\n")); err != nil {
										return handleStdoutError(err)
									}

									return nil
								},
							},
							{
								Name:        "update",
								Usage:       "Update an existing destination",
								Description: "Update an existing destination on the server.",
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "id",
										Usage: "ID of the destination to update",
									},
									&cli.StringFlag{
										Name:    "name",
										Usage:   "Name of the destination",
										Aliases: []string{"n"},
									},
									&cli.StringFlag{
										Name:    "url",
										Usage:   "RTMP URL of the destination",
										Aliases: []string{"u"},
									},
								},
								Action: func(ctx context.Context, c *cli.Command) error {
									destID := idFromArgOrFlag(c)
									if destID == "" {
										return errIDMissing
									}

									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									var name, url optional.V[string]
									if c.IsSet("name") {
										name = optional.New(c.String("name"))
									}
									if c.IsSet("url") {
										url = optional.New(c.String("url"))
									}

									if err := client.UpdateDestination(ctx, destID, name, url); err != nil {
										return fmt.Errorf("update destination: %w", err)
									}

									if _, err := stdout.Write([]byte("OK\n")); err != nil {
										return handleStdoutError(err)
									}

									return nil
								},
							},
							{
								Name:        "remove",
								Usage:       "Remove a destination",
								Description: "Remove a destination on the server.",
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "id",
										Usage: "ID of the destination to remove",
									},
									&cli.BoolFlag{
										Name:    "force",
										Usage:   "Force remove the destination even if it is live",
										Aliases: []string{"f"},
									},
								},
								Action: func(ctx context.Context, c *cli.Command) error {
									destID := idFromArgOrFlag(c)
									if destID == "" {
										return errIDMissing
									}

									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									if err := client.RemoveDestination(ctx, destID, c.Bool("force")); err != nil {
										return fmt.Errorf("remove destination: %w", err)
									}

									if _, err := stdout.Write([]byte("OK\n")); err != nil {
										return handleStdoutError(err)
									}

									return nil
								},
							},
							{
								Name:        "start",
								Usage:       "Start streaming to a destination",
								Description: "Start streaming to a destination.",
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "id",
										Usage: "ID of the destination to start",
									},
								},
								Action: func(ctx context.Context, c *cli.Command) error {
									destID := idFromArgOrFlag(c)
									if destID == "" {
										return errIDMissing
									}

									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									if err := client.StartDestination(ctx, destID); err != nil {
										return fmt.Errorf("start destination: %w", err)
									}

									if _, err := stdout.Write([]byte("OK\n")); err != nil {
										return handleStdoutError(err)
									}

									return nil
								},
							},
							{
								Name:        "stop",
								Usage:       "Stop streaming to a destination",
								Description: "Stop streaming to a destination.",
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "id",
										Usage: "ID of the destination to stop",
									},
								},
								Action: func(ctx context.Context, c *cli.Command) error {
									destID := idFromArgOrFlag(c)
									if destID == "" {
										return errIDMissing
									}

									client, err := buildClient(ctx, c)
									if err != nil {
										return fmt.Errorf("build client: %w", err)
									}

									if err := client.StopDestination(ctx, destID); err != nil {
										return fmt.Errorf("stop destination: %w", err)
									}

									if _, err := stdout.Write([]byte("OK\n")); err != nil {
										return handleStdoutError(err)
									}

									return nil
								},
							},
						},
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					logger, err := buildClientLogger(c)
					if err != nil {
						return fmt.Errorf("build logger: %w", err)
					}

					return runClient(ctx, c, logger)
				},
			},
			{
				Name:  "version",
				Usage: "Display the currrent version",
				Action: func(context.Context, *cli.Command) error {
					if err := printVersion(); err != nil {
						return fmt.Errorf("print version: %s", err)
					}

					return nil
				},
			},
		},
	}

	if err := app.Run(ctx, args); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck

		var authErr x509.UnknownAuthorityError
		if errors.As(err, &authErr) {
			stderr.Write([]byte("Hint: Run with --tls-skip-verify to ignore.\n")) //nolint:errcheck
		}

		return err
	}

	return nil
}

var errIDMissing = errors.New("destination ID is required, either as a the first positional argument or via --id flag")

func idFromArgOrFlag(c *cli.Command) string {
	return cmp.Or(c.Args().First(), c.String("id"))
}

func serverFlags(clientAndServerMode bool) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "listen",
			Usage:       "The address to listen on (plain-text HTTP/gRPC, insecure)",
			DefaultText: defaultListenAddr,
			Category:    "Server",
			Aliases:     []string{"l"},
			Sources:     cli.EnvVars("OCTO_LISTEN"),
			Hidden:      clientAndServerMode,
		},
		&cli.StringFlag{
			Name:        "listen-tls",
			Usage:       "The address to listen on (TLS, self-signed certificate by default)",
			DefaultText: defaultListenAddrTLS,
			Category:    "Server",
			Aliases:     []string{"a"},
			Sources:     cli.EnvVars("OCTO_LISTEN_TLS"),
			Destination: &serverListenAddr,
			Hidden:      clientAndServerMode,
		},
		&cli.StringFlag{
			Name:        "server-url",
			Usage:       "The server URL for web and API clients to connect to",
			DefaultText: defaultServerURL,
			Category:    "Server",
			Aliases:     []string{"u"},
			Sources:     cli.EnvVars("OCTO_SERVER_URL"),
			Destination: &serverURL,
			Hidden:      clientAndServerMode,
		},
		&cli.BoolFlag{
			Name:        "web",
			Usage:       "Enable the web admin interface",
			DefaultText: "true",
			Category:    "Server",
			Aliases:     []string{"w"},
			Sources:     cli.EnvVars("OCTO_WEB"),
			Destination: &webEnabled,
			Hidden:      clientAndServerMode,
		},
		&cli.StringFlag{
			Name:     "docker-host",
			Usage:    "The Docker host to connect to, e.g. ssh://user@host:2375. If not set, falls back to the Docker SDK's DOCKER_HOST environment variable or the default Unix socket.",
			Category: "Server",
			Sources:  cli.EnvVars("OCTO_DOCKER_HOST"),
			Hidden:   clientAndServerMode,
		},
		&cli.StringFlag{
			Name:        "auth",
			Usage:       "Authentication mode for the server",
			Category:    "Server",
			DefaultText: "auto",
			Sources:     cli.EnvVars("OCTO_AUTH"),
			Destination: &serverAuthMode,
			Hidden:      clientAndServerMode,
		},
		&cli.BoolFlag{
			Name:        "insecure-allow-no-auth",
			Usage:       "DANGER: Allow unauthenticated access to the server.",
			Category:    "Server",
			DefaultText: "false",
			Sources:     cli.EnvVars("OCTO_INSECURE_ALLOW_NO_AUTH"),
			Destination: &insecureAllowNoAuth,
			Hidden:      clientAndServerMode,
		},
		&cli.StringFlag{
			Name:      "tls-cert",
			Usage:     "Path to a TLS certificate",
			Category:  "Server",
			TakesFile: true,
			Sources:   cli.EnvVars("OCTO_TLS_CERT"),
		},
		&cli.StringFlag{
			Name:      "tls-key",
			Usage:     "Path to a TLS key",
			Category:  "Server",
			TakesFile: true,
			Sources:   cli.EnvVars("OCTO_TLS_KEY"),
		},
		&cli.BoolFlag{
			Name:     "log-to-file",
			Usage:    "Write logs to file instead of stderr",
			Category: "Logs",
			Sources:  cli.EnvVars("OCTO_LOG_TO_FILE"),
		},
		&cli.StringFlag{
			Name:      "log-file",
			Usage:     "Path to the log file (implies log-to-file)",
			Category:  "Logs",
			TakesFile: true,
			Sources:   cli.EnvVars("OCTO_LOG_FILE"),
		},
		&cli.StringFlag{
			Name:        "log-level",
			Usage:       "Set the logging level",
			Category:    "Logs",
			DefaultText: "info",
			Sources:     cli.EnvVars("OCTO_LOG_LEVEL"),
		},
		&cli.StringFlag{
			Name:        "stream-key",
			Usage:       "Stream key for RTMP sources",
			Category:    "Sources",
			DefaultText: defaultStreamKey,
			Sources:     cli.EnvVars("OCTO_STREAM_KEY"),
		},
		&cli.BoolFlag{
			Name:        "rtmp-enabled",
			Usage:       "Enable the RTMP source",
			Category:    "Sources",
			DefaultText: "true",
		},
		&cli.StringFlag{
			Name:        "rtmp-listen",
			Usage:       "The address to listen on for RTMP",
			Category:    "Sources",
			DefaultText: "127.0.0.1:1935",
		},
		&cli.BoolFlag{
			Name:        "rtmps-enabled",
			Usage:       "Enable the RTMPS source",
			Category:    "Sources",
			DefaultText: "true",
		},
		&cli.StringFlag{
			Name:        "rtmps-listen",
			Usage:       "The address to listen on for RTMPS",
			Category:    "Sources",
			DefaultText: "127.0.0.1:1936",
		},
		flagDataDir(),
		&cli.StringFlag{
			Name:        "image-name-mediamtx",
			Usage:       "OCI-compatible image name for the MediaMTX server",
			Category:    "General",
			DefaultText: mediaserver.DefaultImageNameMediaMTX,
			Sources:     cli.EnvVars("OCTO_IMAGE_NAME_MEDIAMTX"),
		},
		&cli.StringFlag{
			Name:        "image-name-ffmpeg",
			Usage:       "OCI-compatible image name for FFmpeg",
			Category:    "General",
			DefaultText: replicator.DefaultImageNameFFMPEG,
			Sources:     cli.EnvVars("OCTO_IMAGE_NAME_FFMPEG"),
		},
	}
}

func flagDataDir() *cli.StringFlag {
	return &cli.StringFlag{
		Name:     "data-dir",
		Usage:    "Path to the data directory for storing state, logs, etc",
		Category: "General",
		DefaultText: func() string {
			switch runtime.GOOS {
			case "darwin":
				return "$HOME/Library/Caches/octoplex/"
			case "windows":
				panic("not implemented")
			default:
				return "$HOME/.local/state/octoplex/"
			}
		}(),
		Sources: cli.EnvVars("OCTO_DATA_DIR"),
	}
}

// runClient runs the client.
func runClient(ctx context.Context, c *cli.Command, logger *slog.Logger) error {
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
		APIToken:           c.String("api-token"),
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

func buildClient(ctx context.Context, c *cli.Command) (*client.App, error) {
	logger, err := buildClientLogger(c)
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, fmt.Errorf("unable to read build info")
	}

	return client.New(client.NewParams{
		ServerAddr:         clientHost,
		InsecureSkipVerify: clientTLSSkipVerify,
		APIToken:           c.String("api-token"),
		ClipboardAvailable: !clipboard.Unsupported,
		BuildInfo: domain.BuildInfo{
			GoVersion: buildInfo.GoVersion,
			Version:   version,
			Commit:    commit,
			Date:      date,
		},
		Logger: logger,
	}), nil
}

// serverConfig holds additional configuration for launching the server.
type serverConfig struct {
	listenerFunc    server.ListenerFunc // override config
	stderrAvailable bool
	handleSigInt    bool
	waitForClient   bool
}

// runServer runs the server.
func runServer(ctx context.Context, c *cli.Command, cfg config.Config, serverCfg serverConfig, logger *slog.Logger) error {
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

	hostOpt := dockerclient.FromEnv
	if cfg.DockerHost != "" {
		hostOpt = dockerclient.WithHost(cfg.DockerHost)
	}

	dockerClient, err := dockerclient.NewClientWithOpts(hostOpt, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	stateStore, err := store.New(filepath.Join(cfg.DataDir, "state.json"))
	if err != nil {
		return fmt.Errorf("new store: %w", err)
	}

	app, err := server.New(server.Params{
		Config:          cfg,
		Store:           stateStore,
		TokenStore:      store.NewTokenStore(cfg.DataDir, logger),
		DockerClient:    dockerClient,
		ListenerTLSFunc: serverCfg.listenerFunc,
		WaitForClient:   serverCfg.waitForClient,
		Logger:          logger,
	})
	if err != nil {
		return fmt.Errorf("new server: %w", err)
	}

	if c.Name == "stop" {
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

		if errors.Is(err, server.ErrAuthenticationCannotBeDisabled) {
			msg := "Running with --auth none is not permitted with a non-loopback listen address.\n" +
				"Either set `--auth token`, or run the server with `--insecure-allow-no-auth` to disable authentication completely."
			return cli.Exit(msg, 2)
		}

		return err
	}

	return nil
}

// runClientAndServer runs client and server in the same process.
func runClientAndServer(ctx context.Context, c *cli.Command) error {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Override CLI flags:
	serverListenAddr = fmt.Sprintf("127.0.0.1:%d", lis.Addr().(*net.TCPAddr).Port) // listen on all interfaces
	serverURL = "http://localhost:8080"                                            // server URL and DNS name
	webEnabled = true                                                              // enable web interface
	serverAuthMode = "none"                                                        // disable authentication
	insecureAllowNoAuth = true                                                     // disable authentication
	clientHost = fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)       // point client at the correct port
	clientTLSSkipVerify = true                                                     // override default TLS verification

	// must be built after overriding flags:
	cfg, err := parseConfig(c)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// must be built after overriding flags:
	logger, err := buildServerLogger(cfg, nil, false)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	errNoErr := errors.New("no error")
	g, ctx := errgroup.WithContext(ctx)

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

	if err := g.Wait(); errors.Is(err, errNoErr) {
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

func parseConfig(c *cli.Command) (config.Config, error) {
	dataDir := c.String("data-dir")
	if dataDir == "" {
		appStateDir, err := xdg.CreateAppStateDir()
		if err != nil {
			return config.Config{}, fmt.Errorf("create app state dir: %w", err)
		}
		dataDir = appStateDir
	}

	var authMode config.AuthMode
	switch serverAuthMode {
	case "", "auto":
		authMode = config.AuthModeAuto
	case "none":
		authMode = config.AuthModeNone
	case "token":
		authMode = config.AuthModeToken
	default:
		return config.Config{}, fmt.Errorf("invalid auth mode: %s", c.String("auth"))
	}

	logToFileEnabled := c.Bool("log-to-file")
	logFile := c.String("log-file")
	logLevel := cmp.Or(c.String("log-level"), "info")

	if !logToFileEnabled && logFile != "" {
		logToFileEnabled = true // enable logging to file if log-file is set
	} else if logToFileEnabled && logFile == "" {
		logFile = filepath.Join(dataDir, "octoplex.log")
	}

	const argNone = "none"
	var listenAddrPlain, listenAddrTLS string
	if addr := c.String("listen"); addr != argNone {
		listenAddrPlain = cmp.Or(addr, defaultListenAddr)
	}
	if addr := c.String("listen-tls"); addr != argNone {
		listenAddrTLS = cmp.Or(addr, defaultListenAddrTLS)
	}

	serverURL, err := config.NewServerURL(cmp.Or(serverURL, defaultServerURL))
	if err != nil {
		return config.Config{}, fmt.Errorf("new server URL: %w", err)
	}

	cfg := config.Config{
		ListenAddrs:         config.ListenAddrs{Plain: listenAddrPlain, TLS: listenAddrTLS},
		ServerURL:           serverURL,
		AuthMode:            authMode,
		InsecureAllowNoAuth: insecureAllowNoAuth,
		DockerHost:          cmp.Or(c.String("docker-host"), os.Getenv("DOCKER_HOST")),
		InDocker:            os.Getenv("OCTO_DOCKER") == "true",
		Web:                 config.Web{Enabled: !c.IsSet("web") || webEnabled},
		DataDir:             dataDir,
		ImageNameFFMPEG:     c.String("image-name-ffmpeg"),
		Logging: config.Logging{
			ToFile: logToFileEnabled,
			Path:   logFile,
			Level:  logLevel,
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
	cfg.Sources.MediaServer.ImageName = c.String("image-name-mediamtx")

	rtmpEnabled := true
	if c.IsSet("rtmp-enabled") {
		rtmpEnabled = c.Bool("rtmp-enabled")
	}
	cfg.Sources.MediaServer.RTMP.Enabled = rtmpEnabled
	if rtmpEnabled {
		if err := parseRTMPConfig(&cfg.Sources.MediaServer.RTMP, c, "rtmp-listen"); err != nil {
			return config.Config{}, fmt.Errorf("parse RTMP: %w", err)
		}
	}

	rtmpsEnabled := true
	if c.IsSet("rtmps-enabled") {
		rtmpsEnabled = c.Bool("rtmps-enabled")
	}
	cfg.Sources.MediaServer.RTMPS.Enabled = rtmpsEnabled
	if rtmpsEnabled {
		if err := parseRTMPConfig(&cfg.Sources.MediaServer.RTMPS, c, "rtmps-listen"); err != nil {
			return config.Config{}, fmt.Errorf("parse RTMP: %w", err)
		}
	}

	return cfg, nil
}

func parseRTMPConfig(cfg *config.RTMPSource, c *cli.Command, arg string) error {
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

func buildServerLogger(cfg config.Config, stderr io.Writer, stderrAvailable bool) (*slog.Logger, error) {
	var w io.Writer
	if stderrAvailable {
		w = stderr
	} else if !cfg.Logging.ToFile {
		w = io.Discard
	} else {
		var err error
		w, err = os.OpenFile(cfg.Logging.GetPath(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("error opening log file: %w", err)
		}
	}

	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: parseLogLevel(cfg.Logging.Level)})), nil
}

func buildClientLogger(c *cli.Command) (*slog.Logger, error) {
	logger := slog.New(slog.DiscardHandler)

	if logfile := c.String("log-file"); logfile != "" {
		fptr, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		logger = slog.New(slog.NewTextHandler(fptr, &slog.HandlerOptions{Level: parseLogLevel(c.String("log-level"))}))
	}

	return logger, nil
}

func handleStdoutError(err error) error {
	_, _ = fmt.Fprintf(os.Stderr, "failed to write to stdout: %s", err)
	return fmt.Errorf("fprintf: %w", err)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
