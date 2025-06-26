package event

import (
	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/google/uuid"
)

type Name string

const (
	EventNameAppStateChanged         Name = "app_state_changed"
	EventNameDestinationsListed      Name = "destinations_listed"
	EventNameListDestinationsFailed  Name = "list_destinations_failed"
	EventNameDestinationAdded        Name = "destination_added"
	EventNameAddDestinationFailed    Name = "add_destination_failed"
	EventNameDestinationUpdated      Name = "destination_updated"
	EventNameUpdateDestinationFailed Name = "update_destination_failed"
	EventNameDestinationStreamExited Name = "destination_stream_exited"
	EventNameDestinationStarted      Name = "destination_started"
	EventNameStartDestinationFailed  Name = "start_destination_failed"
	EventNameDestinationStopped      Name = "destination_stopped"
	EventNameStopDestinationFailed   Name = "stop_destination_failed"
	EventNameDestinationRemoved      Name = "destination_removed"
	EventNameRemoveDestinationFailed Name = "remove_destination_failed"
	EventNameFatalErrorOccurred      Name = "fatal_error_occurred"
	EventNameOtherInstanceDetected   Name = "other_instance_detected"
	EventNameMediaServerStarted      Name = "media_server_started"
)

// Event represents something which happened in the appllication.
type Event interface {
	EventName() Name
}

// AppStateChangedEvent is emitted when the application state changes.
type AppStateChangedEvent struct {
	State domain.AppState
}

func (e AppStateChangedEvent) EventName() Name {
	return EventNameAppStateChanged
}

// DestinationsListedEvent is emitted when the list of destinations is
// successfully retrieved.
type DestinationsListedEvent struct {
	Destinations []domain.Destination
}

func (e DestinationsListedEvent) EventName() Name {
	return EventNameDestinationsListed
}

// ListDestinationsFailedEvent is emitted when the list of destinations fails
// to be retrieved.
type ListDestinationsFailedEvent struct {
	Err error
}

func (e ListDestinationsFailedEvent) EventName() Name {
	return EventNameListDestinationsFailed
}

// DestinationAddedEvent is emitted when a destination is successfully added.
type DestinationAddedEvent struct {
	ID uuid.UUID
}

func (e DestinationAddedEvent) EventName() Name {
	return EventNameDestinationAdded
}

// AddDestinationFailedEvent is emitted when a destination fails to be added.
type AddDestinationFailedEvent struct {
	URL string
	Err error
}

func (e AddDestinationFailedEvent) EventName() Name {
	return EventNameAddDestinationFailed
}

// DestinationUpdatedEvent is emitted when a destination is successfully updated.
type DestinationUpdatedEvent struct {
	ID uuid.UUID
}

func (e DestinationUpdatedEvent) EventName() Name {
	return EventNameDestinationUpdated
}

// UpdateDestinationFailedEvent is emitted when a destination fails to be updated.
type UpdateDestinationFailedEvent struct {
	ID  uuid.UUID
	Err error
}

func (e UpdateDestinationFailedEvent) EventName() Name {
	return EventNameUpdateDestinationFailed
}

// DestinationStreamExitedEvent is emitted when a destination goes off-air unexpectedly.
type DestinationStreamExitedEvent struct {
	ID   uuid.UUID
	Name string
	Err  error
}

func (e DestinationStreamExitedEvent) EventName() Name {
	return EventNameDestinationStreamExited
}

// DestinationStartedEvent is emitted when a destination starts successfully.
type DestinationStartedEvent struct {
	ID uuid.UUID
}

func (e DestinationStartedEvent) EventName() Name {
	return EventNameDestinationStarted
}

// StartDestinationFailedEvent is emitted when a destination fails to start.
type StartDestinationFailedEvent struct {
	ID  uuid.UUID
	Err error
}

func (e StartDestinationFailedEvent) EventName() Name {
	return EventNameStartDestinationFailed
}

// DestinationStoppedEvent is emitted when a destination stops successfully.
type DestinationStoppedEvent struct {
	ID uuid.UUID
}

func (e DestinationStoppedEvent) EventName() Name {
	return EventNameDestinationStopped
}

// StopDestinationFailedEvent is emitted when a destination fails to stop.
type StopDestinationFailedEvent struct {
	ID  uuid.UUID
	Err error
}

func (e StopDestinationFailedEvent) EventName() Name {
	return EventNameStopDestinationFailed
}

// DestinationRemovedEvent is emitted when a destination is successfully
// removed.
type DestinationRemovedEvent struct {
	ID uuid.UUID
}

func (e DestinationRemovedEvent) EventName() Name {
	return EventNameDestinationRemoved
}

// RemoveDestinationFailedEvent is emitted when a destination fails to be
// removed.
type RemoveDestinationFailedEvent struct {
	ID  uuid.UUID
	Err error
}

func (e RemoveDestinationFailedEvent) EventName() Name {
	return EventNameRemoveDestinationFailed
}

// FatalErrorOccurredEvent is emitted when a fatal application
// error occurs.
type FatalErrorOccurredEvent struct {
	Message string
}

// OtherInstanceDetectedEvent is emitted when the app launches and detects another instance.
type OtherInstanceDetectedEvent struct{}

func (e OtherInstanceDetectedEvent) EventName() Name {
	return EventNameOtherInstanceDetected
}

func (e FatalErrorOccurredEvent) EventName() Name {
	return "fatal_error_occurred"
}

// MediaServerStartedEvent is emitted when the mediaserver component starts successfully.
type MediaServerStartedEvent struct{}

func (e MediaServerStartedEvent) EventName() Name {
	return "media_server_started"
}
