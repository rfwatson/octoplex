package config

import (
	"cmp"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
)

const (
	// TLSMinVersion is the minimum required version of TLS.
	TLSMinVersion = tls.VersionTLS13
)

// Logging holds logging configuration.
type Logging struct {
	ToFile bool   // ToFile is true if logs should be written to file.
	Path   string // Path is an explicit filesystem path to log to. May be zero value if unset.
	Level  string // Level is "debug", "info", etc.

	defaultPath string
}

// GetPath returns the path to the log file. If the path is not set, it
// returns the default log path.
func (l Logging) GetPath() string {
	return cmp.Or(l.Path, l.defaultPath)
}

// NetAddr holds an IP and/or port.
type NetAddr struct {
	IP   string
	Port int
}

// Endpoint holds the configuration for a network endpoint that may be
// enabled or disabled.
type Endpoint struct {
	NetAddr

	Enabled bool
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
	RTMP      Endpoint
	RTMPS     Endpoint
	RTSP      Endpoint
	RTSPS     Endpoint
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

// ServerURL holds the parsed components of a server URL.
type ServerURL struct {
	Raw      string // Raw is the original URL string, e.g. "https://example.com:443/"
	Scheme   string // Scheme is the HTTP scheme, e.g. "http://" or "https://"
	Hostname string // Hostname is the domain part of the host, e.g. "example.com"
	Port     string // Port is the stringified port, e.g. "443" or "80"
	BaseURL  string // Base URL is the base URL for web component, including trailing slash.
}

// NewServerURL parses a raw URL string and returns a ServerURL struct.
func NewServerURL(raw string) (ServerURL, error) {
	if raw == "" {
		return ServerURL{}, fmt.Errorf("empty URL")
	}

	uri, err := url.Parse(raw)
	if err != nil {
		return ServerURL{}, fmt.Errorf("invalid server URL: %w", err)
	}

	if uri.Hostname() == "" {
		return ServerURL{}, fmt.Errorf("invalid server URL: no hostname found")
	}

	if uri.Scheme != "http" && uri.Scheme != "https" {
		return ServerURL{}, fmt.Errorf("invalid server URL: unsupported or missing scheme (must be http or https)")
	}

	baseURL := raw
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	return ServerURL{
		Raw:      raw,
		Scheme:   uri.Scheme,
		Hostname: uri.Hostname(),
		Port:     uri.Port(),
		BaseURL:  baseURL,
	}, nil
}

// Web holds the configuration for the web interface.
type Web struct {
	Enabled bool // Enabled is true if the web interface is enabled.
}

// Config holds the configuration for the application.
type Config struct {
	ListenAddrs         ListenAddrs
	ServerURL           ServerURL
	TLS                 *TLS
	AuthMode            AuthMode
	InsecureAllowNoAuth bool   // DANGER: no authentication even for non-loopback addresses.
	DockerHost          string // DockerHost is the host to connect to the Docker daemon, falls back to Docker SDK's FromEnv(). Empty if not explicitly set.
	InDocker            bool
	Web                 Web
	Debug               bool // deprecated
	DataDir             string
	Logging             Logging
	Sources             Sources
	ImageNameFFMPEG     string
}
