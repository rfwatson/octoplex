package config

import (
	"cmp"
	"crypto/tls"
)

const (
	// TLSMinVersion is the minimum required version of TLS.
	TLSMinVersion = tls.VersionTLS13
)

// LogFile holds the configuration for the log file.
type LogFile struct {
	Enabled bool
	Path    string

	defaultPath string
}

// GetPath returns the path to the log file. If the path is not set, it
// returns the default log path.
func (l LogFile) GetPath() string {
	return cmp.Or(l.Path, l.defaultPath)
}

// NetAddr holds an IP and/or port.
type NetAddr struct {
	IP   string
	Port int
}

// RTMPSource holds the configuration for the RTMP source.
type RTMPSource struct {
	Enabled bool

	NetAddr
}

// TLS holds the TLS configuration.
type TLS struct {
	CertPath string
	KeyPath  string
}

// MediaServerSource holds the configuration for the media server source.
type MediaServerSource struct {
	StreamKey string
	RTMP      RTMPSource
	RTMPS     RTMPSource
}

// Sources holds the configuration for the sources.
type Sources struct {
	MediaServer MediaServerSource
}

// Config holds the configuration for the application.
type Config struct {
	ListenAddr string
	Host       string
	TLS        *TLS
	InDocker   bool
	Debug      bool // deprecated
	LogFile    LogFile
	Sources    Sources
}
