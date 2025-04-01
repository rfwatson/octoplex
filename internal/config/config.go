package config

// Destination holds the configuration for a destination.
type Destination struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// LogFile holds the configuration for the log file.
type LogFile struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

// RTMPSource holds the configuration for the RTMP source.
type RTMPSource struct {
	Enabled   bool   `yaml:"enabled"`
	StreamKey string `yaml:"streamkey,omitempty"`
}

// Sources holds the configuration for the sources.
type Sources struct {
	RTMP RTMPSource `yaml:"rtmp"`
}

// Config holds the configuration for the application.
type Config struct {
	LogFile      LogFile       `yaml:"logfile"`
	Sources      Sources       `yaml:"sources"`
	Destinations []Destination `yaml:"destinations"`
}
