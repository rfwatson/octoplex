package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"git.netflux.io/rob/octoplex/internal/domain"
)

func createAppConfigDir(configDir string) (string, error) {
	path := filepath.Join(configDir, domain.AppName)
	if err := os.MkdirAll(path, 0744); err != nil {
		return "", fmt.Errorf("mkdir all: %w", err)
	}

	return path, nil
}

func createAppStateDir() (string, error) {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(userHomeDir, "/Library", "Caches", domain.AppName)
	case "windows":
		// TODO: Windows support
		return "", errors.New("not implemented")
	default: // Unix-like
		dir = filepath.Join(userHomeDir, ".state", domain.AppName)
	}

	if err := os.MkdirAll(dir, 0744); err != nil {
		return "", fmt.Errorf("mkdir all: %w", err)
	}

	return dir, nil
}
