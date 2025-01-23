package terminal

// CommandToggleDestination toggles a destination from on-air to off-air, or
// vice versa.
type CommandToggleDestination struct {
	URL string
}

// Name implements the Command interface.
func (c CommandToggleDestination) Name() string {
	return "toggle_destination"
}

// Command is an interface for commands that can be triggered by the terminal
// user interface.
type Command interface {
	Name() string
}
