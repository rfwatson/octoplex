package config

import "cmp"

// Destination holds the configuration for a destination.
type Destination struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// LogFile holds the configuration for the log file.
type LogFile struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`

	defaultPath string
}

// GetPath returns the path to the log file. If the path is not set, it
// returns the default log path.
func (l LogFile) GetPath() string {
	return cmp.Or(l.Path, l.defaultPath)
}

// NetAddr holds an IP and/or port.
type NetAddr struct {
	IP   string `yaml:"ip,omitempty"`
	Port int    `yaml:"port,omitempty"`
}

// RTMPSource holds the configuration for the RTMP source.
type RTMPSource struct {
	Enabled bool `yaml:"enabled"`

	NetAddr `yaml:",inline"`
}

// TLS holds the TLS configuration.
type TLS struct {
	CertPath string `yaml:"cert,omitempty"`
	KeyPath  string `yaml:"key,omitempty"`
}

// MediaServerSource holds the configuration for the media server source.
type MediaServerSource struct {
	StreamKey string     `yaml:"streamKey,omitempty"`
	Host      string     `yaml:"host,omitempty"`
	TLS       *TLS       `yaml:"tls,omitempty"`
	RTMP      RTMPSource `yaml:"rtmp"`
	RTMPS     RTMPSource `yaml:"rtmps"`
}

// Sources holds the configuration for the sources.
type Sources struct {
	MediaServer MediaServerSource `yaml:"mediaServer"`
}

// Config holds the configuration for the application.
type Config struct {
	LogFile      LogFile       `yaml:"logfile"`
	Sources      Sources       `yaml:"sources"`
	Destinations []Destination `yaml:"destinations"`
}
