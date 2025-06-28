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
	ImageName string // ImageName is the Docker image of the MediaMTX server.
	StreamKey string
	RTMP      RTMPSource
	RTMPS     RTMPSource
}

// Sources holds the configuration for the sources.
type Sources struct {
	MediaServer MediaServerSource
}

// AuthMode defines the authentication mode for the API.
type AuthMode string

const (
	AuthModeAuto  AuthMode = "auto"
	AuthModeNone  AuthMode = "none"
	AuthModeToken AuthMode = "token"
)

// ListenAddrs holds the listen addresses for the application.
type ListenAddrs struct {
	TLS   string // may be empty
	Plain string // may be empty
}

// Config holds the configuration for the application.
type Config struct {
	ListenAddrs         ListenAddrs
	Host                string
	TLS                 *TLS
	AuthMode            AuthMode
	InsecureAllowNoAuth bool // DANGER: no authentication even for non-loopback addresses.
	InDocker            bool
	Debug               bool // deprecated
	DataDir             string
	LogFile             LogFile
	Sources             Sources
	ImageNameFFMPEG     string
}
