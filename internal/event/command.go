package event

import (
	"git.netflux.io/rob/octoplex/internal/optional"
	"github.com/google/uuid"
)

// CommandListDestinations lists all destinations.
type CommandListDestinations struct{}

// Name implements the Command interface.
func (c CommandListDestinations) Name() string {
	return "list_destinations"
}

// CommandAddDestination adds a destination.
type CommandAddDestination struct {
	DestinationName string
	URL             string
}

// Name implements the Command interface.
func (c CommandAddDestination) Name() string {
	return "add_destination"
}

// CommandUpdateDestination updates a destination.
type CommandUpdateDestination struct {
	ID              uuid.UUID
	DestinationName optional.V[string]
	URL             optional.V[string]
}

// Name implements the Command interface.
func (c CommandUpdateDestination) Name() string {
	return "update_destination"
}

// CommandRemoveDestination removes a destination.
type CommandRemoveDestination struct {
	ID    uuid.UUID
	Force bool // Remove the destination even if it is running.
}

// Name implements the Command interface.
func (c CommandRemoveDestination) Name() string {
	return "remove_destination"
}

// CommandStartDestination starts a destination.
type CommandStartDestination struct {
	ID uuid.UUID
}

// Name implements the Command interface.
func (c CommandStartDestination) Name() string {
	return "start_destination"
}

// CommandStopDestination stops a destination.
type CommandStopDestination struct {
	ID uuid.UUID
}

// Name implements the Command interface.
func (c CommandStopDestination) Name() string {
	return "stop_destination"
}

// CommandCloseOtherInstance closes the other instance of the application.
type CommandCloseOtherInstance struct{}

// Name implements the Command interface.
func (c CommandCloseOtherInstance) Name() string {
	return "close_other_instance"
}

// CommandKillServer kills the server.
type CommandKillServer struct{}

// Name implements the Command interface.
func (c CommandKillServer) Name() string {
	return "kill_server"
}

// Command is an interface for commands that can be triggered by the terminal
// user interface.
type Command interface {
	Name() string
}
