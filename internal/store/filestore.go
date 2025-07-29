package store

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/google/uuid"
)

// Destination represents a destination for a stream.
type Destination struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	URL  string    `json:"url"`
}

// State is the storable persistent state.
type State struct {
	Destinations []Destination `json:"destinations"`
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
func New(path string) (*FileStore, error) {
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

	if validationErrors := validate(state); len(validationErrors) > 0 {
		return nil, fmt.Errorf("stored state is invalid: %v", collectIter(maps.Keys(validationErrors)))
	}

	return &FileStore{path: path, state: &state}, nil
}

// Get returns the current state of the store.
func (s *FileStore) Get() State {
	return *s.state
}

// Set sets the state of the store to the provided state.
func (s *FileStore) Set(state State) (domain.ValidationErrors, error) {
	if validationErrors := validate(state); len(validationErrors) > 0 {
		return validationErrors, nil
	}

	bytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(s.path, bytes, 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	s.state = &state

	return nil, nil
}

// TODO: validate URL format
// TODO: this doesn't really belong here, move somewhere else
func validate(state State) domain.ValidationErrors {
	errs := make(domain.ValidationErrors)
	urlCounts := make(map[string]int)

	for _, dest := range state.Destinations {
		if len(strings.TrimSpace(dest.Name)) == 0 {
			errs.Append("name", "Name cannot be empty")
		}

		if u, urlErr := url.Parse(dest.URL); urlErr != nil {
			errs.Append("url", "URL is invalid")
		} else if u.Scheme != "rtmp" {
			errs.Append("url", "URL must be an RTMP URL")
		} else if strings.TrimSpace(u.Hostname()) == "" {
			errs.Append("url", "URL hostname must not be empty")
		}

		urlCounts[dest.URL]++
	}

	for _, count := range urlCounts {
		if count > 1 {
			errs.Append("url", "URL is a duplicate of another URL")
			break
		}
	}

	return errs
}

func collectIter[V any](seq iter.Seq[V]) []V {
	var out []V
	seq(func(v V) bool {
		out = append(out, v)
		return true
	})
	return out
}
