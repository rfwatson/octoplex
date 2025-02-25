package config

import "git.netflux.io/rob/octoplex/domain"

const defaultLogFile = domain.AppName + ".log"

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
