package domain

// AppState holds application state.
type AppState struct {
	Source       Source
	Destinations []Destination
	Containers   map[string]Container
}

// Source represents the source, currently always the mediaserver.
type Source struct {
	ContainerID string
	Live        bool
	URL         string
}

// Destination is a single destination.
type Destination struct {
	URL string
}

// Container represents the current state of an individual container.
//
// The source of truth is always the Docker daemon, this struct is used only
// for passing asynchronous state.
type Container struct {
	ID               string
	HealthState      string
	CPUPercent       float64
	MemoryUsageBytes uint64
}
