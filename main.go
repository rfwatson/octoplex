package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"

	"git.netflux.io/rob/octoplex/app"
	"git.netflux.io/rob/octoplex/config"
	"git.netflux.io/rob/octoplex/domain"
	dockerclient "github.com/docker/docker/client"
	"golang.design/x/clipboard"
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
			// TODO: improve help message
			flag.Usage()
			return nil
		}
	}

	cfg, err := configService.ReadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("read or create config: %w", err)
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

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("read build info: %w", err)
	}

	return app.Run(
		ctx,
		app.RunParams{
			Config:             cfg,
			DockerClient:       dockerClient,
			ClipboardAvailable: clipboardAvailable,
			BuildInfo: domain.BuildInfo{
				GoVersion: buildInfo.GoVersion,
				Version:   buildInfo.Main.Version,
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
	fmt.Fprintf(os.Stderr, "%s version %s\n", domain.AppName, "0.0.0")
	return nil
}
