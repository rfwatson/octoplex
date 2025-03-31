package config

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"git.netflux.io/rob/octoplex/internal/domain"
	"gopkg.in/yaml.v3"
)

//go:embed data/config.example.yml
var exampleConfig []byte

// Service provides configuration services.
type Service struct {
	current      Config
	appConfigDir string
	appStateDir  string
	configC      chan Config
}

// ConfigDirFunc is a function that returns the user configuration directory.
type ConfigDirFunc func() (string, error)

// defaultChanSize is the default size of the configuration channel.
const defaultChanSize = 64

// NewDefaultService creates a new service with the default configuration file
// location.
func NewDefaultService() (*Service, error) {
	return NewService(os.UserConfigDir, defaultChanSize)
}

// NewService creates a new service with provided ConfigDirFunc.
//
// The app data directories (config and state) are created if they do not
// exist.
func NewService(configDirFunc ConfigDirFunc, chanSize int) (*Service, error) {
	configDir, err := configDirFunc()
	if err != nil {
		return nil, fmt.Errorf("user config dir: %w", err)
	}

	appConfigDir, err := createAppConfigDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("app config dir: %w", err)
	}

	appStateDir, err := createAppStateDir()
	if err != nil {
		return nil, fmt.Errorf("app state dir: %w", err)
	}

	svc := &Service{
		appConfigDir: appConfigDir,
		appStateDir:  appStateDir,
		configC:      make(chan Config, chanSize),
	}

	svc.setDefaults(&svc.current)

	return svc, nil
}

// Current returns the current configuration.
//
// This will be the last-loaded or last-updated configuration, or a default
// configuration if nothing else is available.
func (s *Service) Current() Config {
	return s.current
}

// C returns a channel that receives configuration updates.
//
// The channel is never closed.
func (s *Service) C() <-chan Config {
	return s.configC
}

// ReadOrCreateConfig reads the configuration from the file or creates it with
// default values.
func (s *Service) ReadOrCreateConfig() (cfg Config, _ error) {
	if _, err := os.Stat(s.Path()); os.IsNotExist(err) {
		return s.writeDefaultConfig()
	} else if err != nil {
		return cfg, fmt.Errorf("stat: %w", err)
	}

	return s.readConfig()
}

// SetConfig sets the configuration to the given value and writes it to the
// file.
func (s *Service) SetConfig(cfg Config) error {
	if err := validate(cfg); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	cfgBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err = s.writeConfig(cfgBytes); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	s.current = cfg
	s.configC <- cfg

	return nil
}

// Path returns the path to the configuration file.
func (s *Service) Path() string {
	return filepath.Join(s.appConfigDir, "config.yaml")
}

func (s *Service) readConfig() (cfg Config, _ error) {
	contents, err := os.ReadFile(s.Path())
	if err != nil {
		return cfg, fmt.Errorf("read file: %w", err)
	}

	if err = yaml.Unmarshal(contents, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal: %w", err)
	}

	s.setDefaults(&cfg)

	if err = validate(cfg); err != nil {
		return cfg, err
	}

	s.current = cfg

	return s.current, nil
}

func (s *Service) writeDefaultConfig() (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(exampleConfig, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal: %w", err)
	}

	if err := s.writeConfig(exampleConfig); err != nil {
		return Config{}, fmt.Errorf("write config: %w", err)
	}

	return cfg, nil
}

func (s *Service) writeConfig(cfgBytes []byte) error {
	if err := os.MkdirAll(s.appConfigDir, 0744); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(s.Path(), cfgBytes, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

func (s *Service) setDefaults(cfg *Config) {
	if cfg.LogFile.Enabled && cfg.LogFile.Path == "" {
		cfg.LogFile.Path = filepath.Join(s.appStateDir, domain.AppName+".log")
	}

	cfg.Sources.RTMP.Enabled = true

	for i := range cfg.Destinations {
		if strings.TrimSpace(cfg.Destinations[i].Name) == "" {
			cfg.Destinations[i].Name = fmt.Sprintf("Stream %d", i+1)
		}
	}
}

// TODO: validate URL format
func validate(cfg Config) error {
	var err error

	urlCounts := make(map[string]int)

	for _, dest := range cfg.Destinations {
		if !strings.HasPrefix(dest.URL, "rtmp://") {
			err = errors.Join(err, fmt.Errorf("destination URL must start with rtmp://"))
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
