package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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

	// TODO: inject StateDirFunc
	appStateDir, err := createAppStateDir()
	if err != nil {
		return nil, fmt.Errorf("app state dir: %w", err)
	}

	svc := &Service{
		appConfigDir: appConfigDir,
		appStateDir:  appStateDir,
		configC:      make(chan Config, chanSize),
	}

	svc.populateConfigOnBuild(&svc.current)

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

	cfgBytes, err := marshalConfig(cfg)
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

	s.populateConfigOnRead(&cfg)
	if err = validate(cfg); err != nil {
		return cfg, err
	}

	s.current = cfg

	return s.current, nil
}

func (s *Service) writeDefaultConfig() (Config, error) {
	var cfg Config
	s.populateConfigOnBuild(&cfg)

	cfgBytes, err := marshalConfig(cfg)
	if err != nil {
		return cfg, fmt.Errorf("marshal: %w", err)
	}

	if err := s.writeConfig(cfgBytes); err != nil {
		return Config{}, fmt.Errorf("write config: %w", err)
	}

	return cfg, nil
}

func marshalConfig(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	return buf.Bytes(), nil
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

// populateConfigOnBuild is called to set default values for a new, empty
// configuration.
//
// This function may set serialized fields to arbitrary values.
func (s *Service) populateConfigOnBuild(cfg *Config) {
	cfg.Sources.MediaServer.StreamKey = "live"
	cfg.Sources.MediaServer.RTMP = RTMPSource{
		Enabled: true,
		NetAddr: NetAddr{IP: "127.0.0.1", Port: 1935},
	}

	s.populateConfigOnRead(cfg)
}

// populateConfigOnRead is called to set default values for a configuration
// read from an existing file.
//
// This function should not update any serialized values, which would be a
// confusing experience for the user.
func (s *Service) populateConfigOnRead(cfg *Config) {
	cfg.LogFile.defaultPath = filepath.Join(s.appStateDir, "octoplex.log")
}

// TODO: validate URL format
func validate(cfg Config) error {
	var err error

	urlCounts := make(map[string]int)

	for _, dest := range cfg.Destinations {
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
