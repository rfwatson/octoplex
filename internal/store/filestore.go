package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"git.netflux.io/rob/octoplex/internal/xdg"
)

// Destination represents a destination for a stream.
type Destination struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// State is the storable persistent state.
type State struct {
	Destinations []Destination `json:"destinations"`
}

// PathFunc returns a path to the store file.
type PathFunc func() (string, error)

// DefaultPath returns the default path to the store file.
//
//   - Linux: ~/.local/state/octoplex/state.json
//   - macOS: ~/Library/Caches/octoplex/state.json
func DefaultPath() (string, error) {
	dir, err := xdg.CreateAppStateDir()
	if err != nil {
		return "", fmt.Errorf("create app state dir: %w", err)
	}

	return filepath.Join(dir, "state.json"), nil
}

// StaticPath returns a PathFunc that always returns the
// provided path.
func StaticPath(path string) PathFunc {
	return func() (string, error) {
		return path, nil
	}
}

// FileStore is a file-based store for persistent application
// state. It will probably be replaced with sqlite3 at some point.
//
// FileStore is not thread-safe and should always be used from a
// single goroutine.
type FileStore struct {
	path  string
	state *State
}

// New creates a new FileStore with the provided config file,
// creating it if it does not exist.
func New(pathFunc PathFunc) (*FileStore, error) {
	path, err := pathFunc()
	if err != nil {
		return nil, fmt.Errorf("build path: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return createFile(path)
	} else if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	return readFile(path)
}

func createFile(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	var state State
	bytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(path, bytes, 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &FileStore{path: path, state: &state}, nil
}

func readFile(path string) (*FileStore, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var state State
	if err = json.Unmarshal(bytes, &state); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if err = validate(state); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &FileStore{path: path, state: &state}, nil
}

// Get returns the current state of the store.
func (s *FileStore) Get() State {
	return *s.state
}

// Set sets the state of the store to the provided state.
func (s *FileStore) Set(state State) error {
	if err := validate(state); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	bytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(s.path, bytes, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	s.state = &state

	return nil
}

// TODO: validate URL format
// TODO: this doesn't really belong here, move somewhere else
func validate(state State) error {
	var err error

	urlCounts := make(map[string]int)

	for _, dest := range state.Destinations {
		if u, urlErr := url.Parse(dest.URL); urlErr != nil {
			err = errors.Join(err, fmt.Errorf("invalid destination URL: %w", urlErr))
		} else if u.Scheme != "rtmp" {
			err = errors.Join(err, errors.New("destination URL must be an RTMP URL"))
		}

		urlCounts[dest.URL]++
	}

	for url, count := range urlCounts {
		if count > 1 {
			err = errors.Join(err, fmt.Errorf("duplicate destination URL: %s", url))
		}
	}

	return err
}
