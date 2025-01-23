package domain

// AppState holds application state.
type AppState struct {
	ContainerRunning bool
	IngressLive      bool
	IngressURL       string
	Destinations     []Destination
}

// Destination is a single destination.
type Destination struct {
	URL string
}
