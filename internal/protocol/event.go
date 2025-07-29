package protocol

import (
	"errors"
	"fmt"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EventToWrappedProto converts an event to a wrapped protobuf message (inside pb.Event).
// Used in the streaming gRPC API.
// Use specific event helper functions for direct conversion to protobuf messages.
func EventToWrappedProto(ev event.Event) *pb.Event {
	switch evt := ev.(type) {
	case event.AppStateChangedEvent:
		return &pb.Event{
			EventType: &pb.Event_AppStateChanged{
				AppStateChanged: AppStateChangedEventToProto(evt),
			},
		}
	case event.DestinationsListedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationsListed{
				DestinationsListed: DestinationsListedEventToProto(evt),
			},
		}
	case event.ListDestinationsFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_ListDestinationsFailed{
				ListDestinationsFailed: ListDestinationsFailedEventToProto(evt),
			},
		}
	case event.DestinationAddedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationAdded{
				DestinationAdded: DestinationAddedEventToProto(evt),
			},
		}
	case event.AddDestinationFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_AddDestinationFailed{
				AddDestinationFailed: AddDestinationFailedEventToProto(evt),
			},
		}
	case event.DestinationUpdatedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationUpdated{
				DestinationUpdated: DestinationUpdatedEventToProto(evt),
			},
		}
	case event.UpdateDestinationFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_UpdateDestinationFailed{
				UpdateDestinationFailed: UpdateDestinationFailedEventToProto(evt),
			},
		}
	case event.DestinationStreamExitedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationStreamExited{
				DestinationStreamExited: DestinationStreamExitedEventToProto(evt),
			},
		}
	case event.DestinationStartedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationStarted{
				DestinationStarted: DestinationStartedEventToProto(evt),
			},
		}
	case event.StartDestinationFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_StartDestinationFailed{
				StartDestinationFailed: StartDestinationFailedEventToProto(evt),
			},
		}
	case event.DestinationStoppedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationStopped{
				DestinationStopped: DestinationStoppedEventToProto(evt),
			},
		}
	case event.StopDestinationFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_StopDestinationFailed{
				StopDestinationFailed: StopDestinationFailedEventToProto(evt),
			},
		}
	case event.DestinationRemovedEvent:
		return &pb.Event{
			EventType: &pb.Event_DestinationRemoved{
				DestinationRemoved: DestinationRemovedEventToProto(evt),
			},
		}
	case event.RemoveDestinationFailedEvent:
		return &pb.Event{
			EventType: &pb.Event_RemoveDestinationFailed{
				RemoveDestinationFailed: RemoveDestinationFailedEventToProto(evt),
			},
		}
	case event.FatalErrorOccurredEvent:
		return &pb.Event{
			EventType: &pb.Event_FatalError{
				FatalError: FatalErrorEventToProto(evt),
			},
		}
	case event.OtherInstanceDetectedEvent:
		return &pb.Event{
			EventType: &pb.Event_OtherInstanceDetected{
				OtherInstanceDetected: OtherInstanceDetectedEventToProto(evt),
			},
		}
	case event.MediaServerStartedEvent:
		return &pb.Event{
			EventType: &pb.Event_MediaServerStarted{
				MediaServerStarted: MediaServerStartedEventToProto(evt),
			},
		}
	default:
		panic(fmt.Sprintf("unknown event type: %T", ev))
	}
}

// DestinationsListedEventToProto converts a DestinationsListedEvent to a protobuf message.
func DestinationsListedEventToProto(evt event.DestinationsListedEvent) *pb.DestinationsListedEvent {
	destinations := make([]*pb.Destination, len(evt.Destinations))
	for i, dest := range evt.Destinations {
		destinations[i] = DestinationToProto(dest)
	}

	return &pb.DestinationsListedEvent{Destinations: destinations}
}

// ListDestinationsFailedEventToProto converts a ListDestinationsFailedEvent to a protobuf message.
func ListDestinationsFailedEventToProto(evt event.ListDestinationsFailedEvent) *pb.ListDestinationsFailedEvent {
	return &pb.ListDestinationsFailedEvent{
		Error: evt.Err.Error(),
	}
}

// AppStateChangedEventToProto converts an AppStateChangedEvent to a protobuf message.
func AppStateChangedEventToProto(evt event.AppStateChangedEvent) *pb.AppStateChangedEvent {
	var liveChangedAt *timestamppb.Timestamp
	if !evt.State.Source.LiveChangedAt.IsZero() {
		liveChangedAt = timestamppb.New(evt.State.Source.LiveChangedAt)
	}

	return &pb.AppStateChangedEvent{
		AppState: &pb.AppState{
			Source: &pb.Source{
				Container:     ContainerToProto(evt.State.Source.Container),
				Live:          evt.State.Source.Live,
				LiveChangedAt: liveChangedAt,
				Tracks:        evt.State.Source.Tracks,
				RtmpUrl:       evt.State.Source.RTMPURL,
				RtmpsUrl:      evt.State.Source.RTMPSURL,
				ExitReason:    evt.State.Source.ExitReason,
			},
			Destinations: DestinationsToProto(evt.State.Destinations),
			BuildInfo: &pb.BuildInfo{
				GoVersion: evt.State.BuildInfo.GoVersion,
				Version:   evt.State.BuildInfo.Version,
				Commit:    evt.State.BuildInfo.Commit,
				Date:      evt.State.BuildInfo.Date,
			},
		},
	}
}

// DestinationAddedEventToProto converts a DestinationAddedEvent to a protobuf message.
func DestinationAddedEventToProto(evt event.DestinationAddedEvent) *pb.DestinationAddedEvent {
	return &pb.DestinationAddedEvent{Id: evt.ID[:]}
}

// AddDestinationFailedEventToProto converts an AddDestinationFailedEvent to a protobuf message.
func AddDestinationFailedEventToProto(evt event.AddDestinationFailedEvent) *pb.AddDestinationFailedEvent {
	var errString string
	if evt.Err != nil {
		errString = evt.Err.Error()
	}

	return &pb.AddDestinationFailedEvent{
		Url:              evt.URL,
		Error:            errString,
		ValidationErrors: validationErrorsToProto(evt.ValidationErrors),
	}
}

// DestinationUpdatedEventToProto converts a DestinationUpdatedEvent to a protobuf message.
func DestinationUpdatedEventToProto(evt event.DestinationUpdatedEvent) *pb.DestinationUpdatedEvent {
	return &pb.DestinationUpdatedEvent{Id: evt.ID[:]}
}

// UpdateDestinationFailedEventToProto converts an UpdateDestinationFailedEvent to a protobuf message.
func UpdateDestinationFailedEventToProto(evt event.UpdateDestinationFailedEvent) *pb.UpdateDestinationFailedEvent {
	var errString string
	if evt.Err != nil {
		errString = evt.Err.Error()
	}

	return &pb.UpdateDestinationFailedEvent{
		Id:               evt.ID[:],
		Error:            errString,
		ValidationErrors: validationErrorsToProto(evt.ValidationErrors),
	}
}

// DestinationStreamExitedEventToProto converts a DestinationStreamExitedEvent to a protobuf message.
func DestinationStreamExitedEventToProto(evt event.DestinationStreamExitedEvent) *pb.DestinationStreamExitedEvent {
	return &pb.DestinationStreamExitedEvent{
		Id:    evt.ID[:],
		Name:  evt.Name,
		Error: evt.Err.Error(),
	}
}

// DestinationStartedEventToProto converts a DestinationStartedEvent to a protobuf message.
func DestinationStartedEventToProto(evt event.DestinationStartedEvent) *pb.DestinationStartedEvent {
	return &pb.DestinationStartedEvent{
		Id: evt.ID[:],
	}
}

// StartDestinationFailedEventToProto converts a StartDestinationFailedEvent to a protobuf message.
func StartDestinationFailedEventToProto(evt event.StartDestinationFailedEvent) *pb.StartDestinationFailedEvent {
	return &pb.StartDestinationFailedEvent{
		Id:    evt.ID[:],
		Error: evt.Err.Error(),
	}
}

// DestinationStoppedEventToProto converts a DestinationStoppedEvent to a protobuf message.
func DestinationStoppedEventToProto(evt event.DestinationStoppedEvent) *pb.DestinationStoppedEvent {
	return &pb.DestinationStoppedEvent{
		Id: evt.ID[:],
	}
}

// StopDestinationFailedEventToProto converts a StopDestinationFailedEvent to a protobuf message.
func StopDestinationFailedEventToProto(evt event.StopDestinationFailedEvent) *pb.StopDestinationFailedEvent {
	return &pb.StopDestinationFailedEvent{
		Id:    evt.ID[:],
		Error: evt.Err.Error(),
	}
}

// DestinationRemovedEventToProto converts a DestinationRemovedEvent to a protobuf message.
func DestinationRemovedEventToProto(evt event.DestinationRemovedEvent) *pb.DestinationRemovedEvent {
	return &pb.DestinationRemovedEvent{
		Id: evt.ID[:],
	}
}

// RemoveDestinationFailedEventToProto converts a RemoveDestinationFailedEvent to a protobuf message.
func RemoveDestinationFailedEventToProto(evt event.RemoveDestinationFailedEvent) *pb.RemoveDestinationFailedEvent {
	return &pb.RemoveDestinationFailedEvent{
		Id:    evt.ID[:],
		Error: evt.Err.Error(),
	}
}

// FatalErrorEventToProto converts a FatalErrorOccurredEvent to a protobuf message.
func FatalErrorEventToProto(evt event.FatalErrorOccurredEvent) *pb.FatalErrorEvent {
	return &pb.FatalErrorEvent{Message: evt.Message}
}

// OtherInstanceDetectedEventToProto converts an OtherInstanceDetectedEvent to a protobuf message.
func OtherInstanceDetectedEventToProto(_ event.OtherInstanceDetectedEvent) *pb.OtherInstanceDetectedEvent {
	return &pb.OtherInstanceDetectedEvent{}
}

// MediaServerStartedEventToProto converts a MediaServerStartedEvent to a protobuf message.
func MediaServerStartedEventToProto(event.MediaServerStartedEvent) *pb.MediaServerStartedEvent {
	return &pb.MediaServerStartedEvent{}
}

// EventFromWrappedProto converts a wrapped protobuf message to an event.
func EventFromWrappedProto(pbEv *pb.Event) (event.Event, error) {
	if pbEv == nil || pbEv.EventType == nil {
		return nil, errors.New("nil or empty pb.Event")
	}

	switch evt := pbEv.EventType.(type) {
	case *pb.Event_AppStateChanged:
		return EventFromAppStateChangedProto(evt.AppStateChanged)
	case *pb.Event_DestinationsListed:
		return EventFromDestinationsListedProto(evt.DestinationsListed)
	case *pb.Event_ListDestinationsFailed:
		return EventFromListDestinationsFailedProto(evt.ListDestinationsFailed)
	case *pb.Event_DestinationAdded:
		return EventFromDestinationAddedProto(evt.DestinationAdded)
	case *pb.Event_AddDestinationFailed:
		return EventFromAddDestinationFailedProto(evt.AddDestinationFailed)
	case *pb.Event_DestinationUpdated:
		return EventFromDestinationUpdatedProto(evt.DestinationUpdated)
	case *pb.Event_UpdateDestinationFailed:
		return EventFromUpdateDestinationFailedProto(evt.UpdateDestinationFailed)
	case *pb.Event_DestinationStreamExited:
		return EventFromDestinationStreamExitedProto(evt.DestinationStreamExited)
	case *pb.Event_DestinationStarted:
		return EventFromDestinationStartedProto(evt.DestinationStarted)
	case *pb.Event_StartDestinationFailed:
		return EventFromStartDestinationFailedProto(evt.StartDestinationFailed)
	case *pb.Event_DestinationStopped:
		return EventFromDestinationStoppedProto(evt.DestinationStopped)
	case *pb.Event_StopDestinationFailed:
		return EventFromStopDestinationFailedProto(evt.StopDestinationFailed)
	case *pb.Event_DestinationRemoved:
		return EventFromDestinationRemovedProto(evt.DestinationRemoved)
	case *pb.Event_RemoveDestinationFailed:
		return EventFromRemoveDestinationFailedProto(evt.RemoveDestinationFailed)
	case *pb.Event_FatalError:
		return EventFromFatalErrorProto(evt.FatalError)
	case *pb.Event_OtherInstanceDetected:
		return EventFromOtherInstanceDetectedProto(evt.OtherInstanceDetected)
	case *pb.Event_MediaServerStarted:
		return EventFromMediaServerStartedProto(evt.MediaServerStarted)
	default:
		return nil, fmt.Errorf("unknown event type: %T", evt)
	}
}

// EventFromDestinationsListedProto converts a protobuf DestinationsListedEvent to a domain event.
func EventFromDestinationsListedProto(evt *pb.DestinationsListedEvent) (event.Event, error) {
	if evt == nil {
		return nil, fmt.Errorf("nil DestinationsListedEvent")
	}

	destinations, err := ProtoToDestinations(evt.Destinations)
	if err != nil {
		return nil, fmt.Errorf("convert destinations: %w", err)
	}

	return event.DestinationsListedEvent{
		Destinations: destinations,
	}, nil
}

// EventFromListDestinationsFailedProto converts a protobuf ListDestinationsFailedEvent to a domain event.
func EventFromListDestinationsFailedProto(evt *pb.ListDestinationsFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, fmt.Errorf("nil ListDestinationsFailedEvent")
	}

	return event.ListDestinationsFailedEvent{
		Err: fmt.Errorf("%s", evt.Error),
	}, nil
}

// EventFromAppStateChangedProto converts a protobuf AppStateChangedEvent to a domain event.
func EventFromAppStateChangedProto(evt *pb.AppStateChangedEvent) (event.Event, error) {
	if evt == nil || evt.AppState == nil || evt.AppState.Source == nil {
		return nil, errors.New("nil or empty AppStateChangedEvent")
	}

	var liveChangedAt time.Time
	if evt.AppState.Source.LiveChangedAt != nil {
		liveChangedAt = evt.AppState.Source.LiveChangedAt.AsTime()
	}

	destinations, err := ProtoToDestinations(evt.AppState.Destinations)
	if err != nil {
		return nil, fmt.Errorf("parse destinations: %w", err)
	}

	return event.AppStateChangedEvent{
		State: domain.AppState{
			Source: domain.Source{
				Container:     ContainerFromProto(evt.AppState.Source.Container),
				Live:          evt.AppState.Source.Live,
				LiveChangedAt: liveChangedAt,
				Tracks:        evt.AppState.Source.Tracks,
				RTMPURL:       evt.AppState.Source.RtmpUrl,
				RTMPSURL:      evt.AppState.Source.RtmpsUrl,
				ExitReason:    evt.AppState.Source.ExitReason,
			},
			Destinations: destinations,
			BuildInfo: domain.BuildInfo{
				GoVersion: evt.AppState.BuildInfo.GoVersion,
				Version:   evt.AppState.BuildInfo.Version,
				Commit:    evt.AppState.BuildInfo.Commit,
				Date:      evt.AppState.BuildInfo.Date,
			},
		},
	}, nil
}

// EventFromDestinationAddedProto converts a protobuf DestinationAddedEvent to a domain event.
func EventFromDestinationAddedProto(evt *pb.DestinationAddedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationAddedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationAddedEvent{ID: id}, nil
}

// EventFromAddDestinationFailedProto converts a protobuf AddDestinationFailedEvent to a domain event.
func EventFromAddDestinationFailedProto(evt *pb.AddDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil AddDestinationFailedEvent")
	}
	var evtErr error
	if evt.Error != "" {
		evtErr = errors.New(evt.Error)
	}

	return event.AddDestinationFailedEvent{
		URL:              evt.Url,
		Err:              evtErr,
		ValidationErrors: validationErrorsFromProto(evt.ValidationErrors),
	}, nil
}

// EventFromDestinationUpdatedProto converts a protobuf DestinationUpdatedEvent to a domain event.
func EventFromDestinationUpdatedProto(evt *pb.DestinationUpdatedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationUpdatedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationUpdatedEvent{ID: id}, nil
}

// EventFromUpdateDestinationFailedProto converts a protobuf UpdateDestinationFailedEvent to a domain event.
func EventFromUpdateDestinationFailedProto(evt *pb.UpdateDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil UpdateDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	var evtErr error
	if evt.Error != "" {
		evtErr = errors.New(evt.Error)
	}

	return event.UpdateDestinationFailedEvent{
		ID:               id,
		Err:              evtErr,
		ValidationErrors: validationErrorsFromProto(evt.ValidationErrors),
	}, nil
}

// EventFromDestinationStreamExitedProto converts a protobuf DestinationStreamExitedEvent to a domain event.
func EventFromDestinationStreamExitedProto(evt *pb.DestinationStreamExitedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationStreamExitedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationStreamExitedEvent{ID: id, Name: evt.Name, Err: errors.New(evt.Error)}, nil
}

// EventFromDestinationStartedProto converts a protobuf DestinationStartedEvent to a domain event.
func EventFromDestinationStartedProto(evt *pb.DestinationStartedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationStartedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationStartedEvent{ID: id}, nil
}

// EventFromStartDestinationFailedProto converts a protobuf StartDestinationFailedEvent to a domain event.
func EventFromStartDestinationFailedProto(evt *pb.StartDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil StartDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.StartDestinationFailedEvent{ID: id, Err: errors.New(evt.Error)}, nil
}

// EventFromDestinationStoppedProto converts a protobuf DestinationStoppedEvent to a domain event.
func EventFromDestinationStoppedProto(evt *pb.DestinationStoppedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationStoppedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationStoppedEvent{ID: id}, nil
}

// EventFromStopDestinationFailedProto converts a protobuf StopDestinationFailedEvent to a domain event.
func EventFromStopDestinationFailedProto(evt *pb.StopDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil StopDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.StopDestinationFailedEvent{ID: id, Err: errors.New(evt.Error)}, nil
}

// EventFromDestinationRemovedProto converts a protobuf DestinationRemovedEvent to a domain event.
func EventFromDestinationRemovedProto(evt *pb.DestinationRemovedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationRemovedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationRemovedEvent{ID: id}, nil
}

// EventFromRemoveDestinationFailedProto converts a protobuf RemoveDestinationFailedEvent to a domain event.
func EventFromRemoveDestinationFailedProto(evt *pb.RemoveDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil RemoveDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.RemoveDestinationFailedEvent{ID: id, Err: errors.New(evt.Error)}, nil
}

// EventFromFatalErrorProto converts a protobuf FatalErrorEvent to a domain event.
func EventFromFatalErrorProto(evt *pb.FatalErrorEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil FatalErrorEvent")
	}
	return event.FatalErrorOccurredEvent{Message: evt.Message}, nil
}

// EventFromOtherInstanceDetectedProto converts a protobuf OtherInstanceDetectedEvent to a domain event.
func EventFromOtherInstanceDetectedProto(_ *pb.OtherInstanceDetectedEvent) (event.Event, error) {
	return event.OtherInstanceDetectedEvent{}, nil
}

// EventFromMediaServerStartedProto converts a protobuf MediaServerStartedEvent to a domain event.
func EventFromMediaServerStartedProto(evt *pb.MediaServerStartedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil MediaServerStartedEvent")
	}
	return event.MediaServerStartedEvent{}, nil
}

func validationErrorsToProto(in domain.ValidationErrors) map[string]*structpb.ListValue {
	if in == nil {
		return nil
	}

	out := make(map[string]*structpb.ListValue)
	for field, validationErrors := range in {
		values := make([]*structpb.Value, 0, len(validationErrors))
		for _, msg := range validationErrors {
			value, _ := structpb.NewValue(msg) // msg is a string, will not error.
			values = append(values, value)
		}
		out[field] = &structpb.ListValue{Values: values}
	}
	return out
}

func validationErrorsFromProto(in map[string]*structpb.ListValue) domain.ValidationErrors {
	if in == nil {
		return nil
	}

	out := make(domain.ValidationErrors)
	for field, listValue := range in {
		if listValue == nil {
			continue
		}
		messages := make([]string, 0, len(listValue.Values))
		for _, value := range listValue.Values {
			if value.GetStringValue() != "" {
				messages = append(messages, value.GetStringValue())
			}
		}
		out[field] = messages
	}
	return out
}
