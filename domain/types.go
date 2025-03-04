package domain

import (
	"slices"
	"time"
)

// AppState holds application state.
type AppState struct {
	Source       Source
	Destinations []Destination
	BuildInfo    BuildInfo
}

// Clone performs a deep copy of AppState.
func (s *AppState) Clone() AppState {
	return AppState{
		Source:       s.Source,
		Destinations: slices.Clone(s.Destinations),
		BuildInfo:    s.BuildInfo,
	}
}

// BuildInfo holds information about the build.
type BuildInfo struct {
	GoVersion string
	Version   string
}

// Source represents the source, currently always the mediaserver.
type Source struct {
	Container       Container
	Live            bool
	LiveChangedAt   time.Time
	Listeners       int
	Tracks          []string
	RTMPURL         string
	RTMPInternalURL string
	ExitReason      string
}

type DestinationStatus int

const (
	DestinationStatusOffAir DestinationStatus = iota
	DestinationStatusStarting
	DestinationStatusLive
)

// Destination is a single destination.
type Destination struct {
	Container Container
	Status    DestinationStatus
	Name      string
	URL       string
}

// Container represents the current state of an individual container.
//
// The source of truth is always the Docker daemon, this struct is used only
// for passing asynchronous state.
type Container struct {
	ID               string
	State            string
	HealthState      string
	CPUPercent       float64
	MemoryUsageBytes uint64
	RxRate           int
	TxRate           int
	RxSince          time.Time
	RestartCount     int
	ExitCode         *int
}
