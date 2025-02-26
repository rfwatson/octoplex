package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.netflux.io/rob/octoplex/domain"
	"gopkg.in/yaml.v3"
)

// Service provides configuration services.
type Service struct {
	configDir string
}

// ConfigDirFunc is a function that returns the user configuration directory.
type ConfigDirFunc func() (string, error)

// NewDefaultService creates a new service with the default configuration file
// location.
func NewDefaultService() (*Service, error) {
	return NewService(os.UserConfigDir)
}

// NewService creates a new service with provided ConfigDirFunc.
func NewService(configDirFunc ConfigDirFunc) (*Service, error) {
	userConfigDir, err := configDirFunc()
	if err != nil {
		return nil, fmt.Errorf("user config dir: %w", err)
	}

	return &Service{
		configDir: filepath.Join(userConfigDir, domain.AppName),
	}, nil
}

// ReadOrCreateConfig reads the configuration from the file at the given path or
// creates it with default values.
func (s *Service) ReadOrCreateConfig() (cfg Config, _ error) {
	if _, err := os.Stat(s.Path()); os.IsNotExist(err) {
		return s.createConfig()
	} else if err != nil {
		return cfg, fmt.Errorf("stat: %w", err)
	}

	return s.readConfig()
}

func (s *Service) readConfig() (cfg Config, _ error) {
	contents, err := os.ReadFile(s.Path())
	if err != nil {
		return cfg, fmt.Errorf("read file: %w", err)
	}

	if err = yaml.Unmarshal(contents, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal: %w", err)
	}

	setDefaults(&cfg)

	if err = validate(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (s *Service) createConfig() (cfg Config, _ error) {
	if err := os.MkdirAll(s.configDir, 0744); err != nil {
		return cfg, fmt.Errorf("mkdir: %w", err)
	}

	setDefaults(&cfg)

	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return cfg, fmt.Errorf("marshal: %w", err)
	}

	if err = os.WriteFile(s.Path(), yamlBytes, 0644); err != nil {
		return cfg, fmt.Errorf("write file: %w", err)
	}

	return cfg, nil
}

func (s *Service) Path() string {
	return filepath.Join(s.configDir, "config.yaml")
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
