package domain

// AppState holds application state.
type AppState struct {
	ContainerRunning bool
	IngressLive      bool
	IngressURL       string
}
