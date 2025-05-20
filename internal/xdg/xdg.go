package xdg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"git.netflux.io/rob/octoplex/internal/domain"
)

// createAppConfigDir creates the application config directory,
// which is at:
//
//   - Linux: ~/.local/state/octoplex
//   - macOS: ~/Library/Caches/octoplex
func CreateAppStateDir() (string, error) {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(userHomeDir, "Library", "Caches", domain.AppName)
	case "windows":
		// TODO: Windows support
		return "", errors.New("not implemented")
	default: // Unix-like
		dir = filepath.Join(userHomeDir, ".local", "state", domain.AppName)
	}

	if err := os.MkdirAll(dir, 0744); err != nil {
		return "", fmt.Errorf("mkdir all: %w", err)
	}

	return dir, nil
}
