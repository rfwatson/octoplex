package event

import "git.netflux.io/rob/octoplex/internal/domain"

type Name string

const (
	EventNameAppStateChanged    Name = "app_state_changed"
	EventNameDestinationAdded   Name = "destination_added"
	EventNameDestinationRemoved Name = "destination_removed"
	EventNameMediaServerStarted Name = "media_server_started"
	EventNameFatalErrorOccurred Name = "fatal_error_occurred"
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

// DestinationRemovedEvent is emitted when a destination is successfully
// removed.
type DestinationRemovedEvent struct {
	URL string
}

func (e DestinationRemovedEvent) name() Name {
	return EventNameDestinationRemoved
}

// MediaServerStartedEvent is emitted when the mediaserver component starts successfully.
type MediaServerStartedEvent struct {
	RTMPURL  string
	RTMPSURL string
}

func (e MediaServerStartedEvent) name() Name {
	return "media_server_started"
}

// FatalErrorOccurredEvent is emitted when a fatal application
// error occurs.
type FatalErrorOccurredEvent struct {
	Message string
}

func (e FatalErrorOccurredEvent) name() Name {
	return "fatal_error_occurred"
}
