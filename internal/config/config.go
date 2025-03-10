package config

// Destination holds the configuration for a destination.
type Destination struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// LogFile holds the configuration for the log file.
type LogFile struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// Config holds the configuration for the application.
type Config struct {
	LogFile      LogFile       `yaml:"logfile"`
	Destinations []Destination `yaml:"destinations"`
}
