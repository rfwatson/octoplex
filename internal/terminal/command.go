package terminal

// CommandStartDestination starts a destination.
type CommandStartDestination struct {
	URL string
}

// Name implements the Command interface.
func (c CommandStartDestination) Name() string {
	return "start_destination"
}

// CommandStopDestination stops a destination.
type CommandStopDestination struct {
	URL string
}

// Name implements the Command interface.
func (c CommandStopDestination) Name() string {
	return "stop_destination"
}

// CommandQuit quits the app.
type CommandQuit struct{}

// Name implements the Command interface.
func (c CommandQuit) Name() string {
	return "quit"
}

// Command is an interface for commands that can be triggered by the terminal
// user interface.
type Command interface {
	Name() string
}
