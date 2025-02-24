package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultLogFile = "octoplex.log"

// Destination holds the configuration for a destination.
type Destination struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Config holds the configuration for the application.
type Config struct {
	LogFile      string        `yaml:"logfile"`
	Destinations []Destination `yaml:"destinations"`
}

// FromFile returns a reader for the default configuration file.
func FromFile() io.Reader {
	r, err := os.Open("config.yml")
	if err != nil {
		return bytes.NewReader([]byte{})
	}

	return r
}

// Default returns a reader for the default configuration.
func Default() io.Reader {
	return bytes.NewReader([]byte(nil))
}

// Load loads the configuration from the given reader.
//
// Passing an empty reader will load the default configuration.
func Load(r io.Reader) (cfg Config, _ error) {
	filePayload, err := io.ReadAll(r)
	if err != nil {
		return cfg, fmt.Errorf("read file: %w", err)
	}

	if err = yaml.Unmarshal(filePayload, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal: %w", err)
	}

	setDefaults(&cfg)

	if err = validate(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.LogFile == "" {
		cfg.LogFile = defaultLogFile
	}

	for i := range cfg.Destinations {
		if strings.TrimSpace(cfg.Destinations[i].Name) == "" {
			cfg.Destinations[i].Name = fmt.Sprintf("Stream %d", i+1)
		}
	}
}

func validate(cfg Config) error {
	var err error

	for _, dest := range cfg.Destinations {
		if !strings.HasPrefix(dest.URL, "rtmp://") {
			err = errors.Join(err, fmt.Errorf("destination URL must start with rtmp://"))
		}
	}

	return err
}
