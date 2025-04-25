package event

import "git.netflux.io/rob/octoplex/internal/domain"

type Name string

const (
	EventNameAppStateChanged         Name = "app_state_changed"
	EventNameDestinationAdded        Name = "destination_added"
	EventNameAddDestinationFailed    Name = "add_destination_failed"
	EventNameDestinationStreamExited Name = "destination_stream_exited"
	EventNameStartDestinationFailed  Name = "start_destination_failed"
	EventNameDestinationRemoved      Name = "destination_removed"
	EventNameRemoveDestinationFailed Name = "remove_destination_failed"
	EventNameFatalErrorOccurred      Name = "fatal_error_occurred"
	EventNameOtherInstanceDetected   Name = "other_instance_detected"
	EventNameMediaServerStarted      Name = "media_server_started"
)

// Event represents something which happened in the appllication.
type Event interface {
	name() Name
}

// AppStateChangedEvent is emitted when the application state changes.
type AppStateChangedEvent struct {
	State domain.AppState
}

func (e AppStateChangedEvent) name() Name {
	return EventNameAppStateChanged
}

// DestinationAddedEvent is emitted when a destination is successfully added.
type DestinationAddedEvent struct {
	URL string
}

func (e DestinationAddedEvent) name() Name {
	return EventNameDestinationAdded
}

// AddDestinationFailedEvent is emitted when a destination fails to be added.
type AddDestinationFailedEvent struct {
	Err error
}

func (e AddDestinationFailedEvent) name() Name {
	return EventNameAddDestinationFailed
}

// DestinationStreamExitedEvent is emitted when a destination goes off-air unexpectedly.
type DestinationStreamExitedEvent struct {
	Name string
	Err  error
}

func (e DestinationStreamExitedEvent) name() Name {
	return EventNameDestinationStreamExited
}

// StartDestinationFailedEvent is emitted when a destination fails to start.
type StartDestinationFailedEvent struct{}

func (e StartDestinationFailedEvent) name() Name {
	return EventNameStartDestinationFailed
}

// DestinationRemovedEvent is emitted when a destination is successfully
// removed.
type DestinationRemovedEvent struct {
	URL string
}

func (e DestinationRemovedEvent) name() Name {
	return EventNameDestinationRemoved
}

// RemoveDestinationFailedEvent is emitted when a destination fails to be
// removed.
type RemoveDestinationFailedEvent struct {
	Err error
}

func (e RemoveDestinationFailedEvent) name() Name {
	return EventNameRemoveDestinationFailed
}

// FatalErrorOccurredEvent is emitted when a fatal application
// error occurs.
type FatalErrorOccurredEvent struct {
	Message string
}

// OtherInstanceDetectedEvent is emitted when the app launches and detects another instance.
type OtherInstanceDetectedEvent struct{}

func (e OtherInstanceDetectedEvent) name() Name {
	return EventNameOtherInstanceDetected
}

func (e FatalErrorOccurredEvent) name() Name {
	return "fatal_error_occurred"
}

// MediaServerStartedEvent is emitted when the mediaserver component starts successfully.
type MediaServerStartedEvent struct {
	RTMPURL  string
	RTMPSURL string
}

func (e MediaServerStartedEvent) name() Name {
	return "media_server_started"
}
