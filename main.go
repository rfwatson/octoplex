package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"syscall"

	"git.netflux.io/rob/octoplex/internal/app"
	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	dockerclient "github.com/docker/docker/client"
	"golang.design/x/clipboard"
)

var (
	// version is the version of the application.
	version string
	// commit is the commit hash of the application.
	commit string
	// date is the date of the build.
	date string
)

var errShutdown = errors.New("shutdown")

func main() {
	var exitStatus int

	if err := run(); errors.Is(err, errShutdown) {
		exitStatus = 130
	} else if err != nil {
		exitStatus = 1
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}

	os.Exit(exitStatus)
}

func run() error {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	configService, err := config.NewDefaultService()
	if err != nil {
		return fmt.Errorf("build config service: %w", err)
	}

	help := flag.Bool("h", false, "Show help")

	flag.Parse()

	if *help {
		printUsage()
		return nil
	}

	if narg := flag.NArg(); narg > 1 {
		printUsage()
		return fmt.Errorf("too many arguments")
	} else if narg == 1 {
		switch flag.Arg(0) {
		case "edit-config":
			return editConfigFile(configService)
		case "print-config":
			return printConfigPath(configService.Path())
		case "version":
			return printVersion()
		case "help":
			printUsage()
			return nil
		}
	}

	cfg, err := configService.ReadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("read or create config: %w", err)
	}

	headless := os.Getenv("OCTO_HEADLESS") != ""
	logger, err := buildLogger(cfg.LogFile, headless)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	if headless {
		// When running in headless mode tview doesn't handle SIGINT for us.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-ch
			logger.Info("Received interrupt signal, exiting")
			signal.Stop(ch)
			cancel(errShutdown)
		}()
	}

	var clipboardAvailable bool
	if err = clipboard.Init(); err != nil {
		logger.Warn("Clipboard not available", "err", err)
	} else {
		clipboardAvailable = true
	}

	dockerClient, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("new docker client: %w", err)
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("read build info: %w", err)
	}

	app := app.New(app.Params{
		ConfigService:      configService,
		DockerClient:       dockerClient,
		Headless:           headless,
		ClipboardAvailable: clipboardAvailable,
		ConfigFilePath:     configService.Path(),
		BuildInfo: domain.BuildInfo{
			GoVersion: buildInfo.GoVersion,
			Version:   version,
			Commit:    commit,
			Date:      date,
		},
		Logger: logger,
	})

	return app.Run(ctx)
}

// editConfigFile opens the config file in the user's editor.
func editConfigFile(configService *config.Service) error {
	if _, err := configService.ReadOrCreateConfig(); err != nil {
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
	fmt.Println(binary)

	if err := syscall.Exec(binary, []string{"--", configService.Path()}, os.Environ()); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	return nil
}

// printConfigPath prints the path to the config file to stderr.
func printConfigPath(configPath string) error {
	fmt.Fprintln(os.Stderr, configPath)
	return nil
}

// printVersion prints the version of the application to stderr.
func printVersion() error {
	fmt.Fprintf(os.Stderr, "%s version %s\n", domain.AppName, cmp.Or(version, "0.0.0-dev"))
	return nil
}

func printUsage() {
	os.Stderr.WriteString("Usage: octoplex [command]\n\n")
	os.Stderr.WriteString("Commands:\n\n")
	os.Stderr.WriteString("  edit-config   Edit the config file\n")
	os.Stderr.WriteString("  print-config  Print the path to the config file\n")
	os.Stderr.WriteString("  version       Print the version of the application\n")
	os.Stderr.WriteString("  help          Print this help message\n")
	os.Stderr.WriteString("\n")
	os.Stderr.WriteString("Additionally, Octoplex can be configured with the following environment variables:\n\n")
	os.Stderr.WriteString("  OCTO_DEBUG    Enables debug logging if set\n")
	os.Stderr.WriteString("  OCTO_HEADLESS Enables headless mode if set (experimental)\n\n")
}

// buildLogger builds the logger, which may be a no-op logger.
func buildLogger(cfg config.LogFile, headless bool) (*slog.Logger, error) {
	build := func(w io.Writer) *slog.Logger {
		var handlerOpts slog.HandlerOptions
		if os.Getenv("OCTO_DEBUG") != "" {
			handlerOpts.Level = slog.LevelDebug
		}
		return slog.New(slog.NewTextHandler(w, &handlerOpts))
	}

	// In headless mode, always log to stderr.
	if headless {
		return build(os.Stderr), nil
	}

	if !cfg.Enabled {
		return slog.New(slog.DiscardHandler), nil
	}

	fptr, err := os.OpenFile(cfg.GetPath(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening log file: %w", err)
	}

	return build(fptr), nil
}
