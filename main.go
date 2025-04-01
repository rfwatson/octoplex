package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		_, _ = os.Stderr.WriteString("Error: " + err.Error() + "\n")
	}
}

func run(ctx context.Context) error {
	configService, err := config.NewDefaultService()
	if err != nil {
		return fmt.Errorf("build config service: %w", err)
	}

	flag.Parse()
	if narg := flag.NArg(); narg > 1 {
		flag.Usage()
		return fmt.Errorf("too many arguments")
	} else if narg == 1 {
		switch flag.Arg(0) {
		case "edit-config":
			return editConfigFile(configService.Path())
		case "print-config":
			return printConfigPath(configService.Path())
		case "version":
			return printVersion()
		case "help", "-h", "--help":
			printUsage()
			return nil
		}
	}

	cfg, err := configService.ReadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("read or create config: %w", err)
	}
	logger, err := buildLogger(cfg.LogFile)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
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

	return app.Run(
		ctx,
		app.RunParams{
			ConfigService:      configService,
			DockerClient:       dockerClient,
			ClipboardAvailable: clipboardAvailable,
			ConfigFilePath:     configService.Path(),
			BuildInfo: domain.BuildInfo{
				GoVersion: buildInfo.GoVersion,
				Version:   version,
				Commit:    commit,
				Date:      date,
			},
			Logger: logger,
		},
	)
}

// editConfigFile opens the config file in the user's editor.
func editConfigFile(configPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	binary, err := exec.LookPath(editor)
	if err != nil {
		return fmt.Errorf("look path: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Editing config file: %s\n", configPath)
	fmt.Println(binary)

	if err := syscall.Exec(binary, []string{"--", configPath}, os.Environ()); err != nil {
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
}

// buildLogger builds the logger, which may be a no-op logger.
func buildLogger(cfg config.LogFile) (*slog.Logger, error) {
	if !cfg.Enabled {
		return slog.New(slog.DiscardHandler), nil
	}

	fptr, err := os.OpenFile(cfg.Path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening log file: %w", err)
	}

	var handlerOpts slog.HandlerOptions
	if os.Getenv("OCTO_DEBUG") != "" {
		handlerOpts.Level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(fptr, &handlerOpts)), nil
}
