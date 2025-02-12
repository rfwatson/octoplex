package domain

import "time"

// AppState holds application state.
type AppState struct {
	Source       Source
	Destinations []Destination
}

// Source represents the source, currently always the mediaserver.
type Source struct {
	Container       Container
	Live            bool
	Listeners       int
	RTMPURL         string
	RTMPInternalURL string
	ExitReason      string
}

type DestinationState int

const (
	DestinationStateOffAir DestinationState = iota
	DestinationStateStarting
	DestinationStateLive
)

// Destination is a single destination.
type Destination struct {
	Container Container
	State     DestinationState
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
