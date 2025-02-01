package domain

// AppState holds application state.
type AppState struct {
	Source       Source
	Destinations []Destination
}

// Source represents the source, currently always the mediaserver.
type Source struct {
	Container Container
	Live      bool
	URL       string
}

// Destination is a single destination.
type Destination struct {
	Container Container
	Live      bool
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
}
