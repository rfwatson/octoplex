package protocol

import (
	"errors"
	"fmt"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EventToProto converts an event to a protobuf message.
func EventToProto(ev event.Event) *pb.Event {
	switch evt := ev.(type) {
	case event.AppStateChangedEvent:
		return buildAppStateChangeEvent(evt)
	case event.DestinationAddedEvent:
		return buildDestinationAddedEvent(evt)
	case event.AddDestinationFailedEvent:
		return buildAddDestinationFailedEvent(evt)
	case event.DestinationStreamExitedEvent:
		return buildDestinationStreamExitedEvent(evt)
	case event.StartDestinationFailedEvent:
		return buildStartDestinationFailedEvent(evt)
	case event.DestinationRemovedEvent:
		return buildDestinationRemovedEvent(evt)
	case event.RemoveDestinationFailedEvent:
		return buildRemoveDestinationFailedEvent(evt)
	case event.FatalErrorOccurredEvent:
		return buildFatalErrorOccurredEvent(evt)
	case event.OtherInstanceDetectedEvent:
		return buildOtherInstanceDetectedEvent(evt)
	case event.MediaServerStartedEvent:
		return buildMediaServerStartedEvent(evt)
	default:
		panic("unknown event type")
	}
}

func buildAppStateChangeEvent(evt event.AppStateChangedEvent) *pb.Event {
	var liveChangedAt *timestamppb.Timestamp
	if !evt.State.Source.LiveChangedAt.IsZero() {
		liveChangedAt = timestamppb.New(evt.State.Source.LiveChangedAt)
	}

	return &pb.Event{
		EventType: &pb.Event_AppStateChanged{
			AppStateChanged: &pb.AppStateChangedEvent{
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
			},
		},
	}
}

func buildDestinationAddedEvent(evt event.DestinationAddedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_DestinationAdded{
			DestinationAdded: &pb.DestinationAddedEvent{Id: evt.ID[:]},
		},
	}
}

func buildAddDestinationFailedEvent(evt event.AddDestinationFailedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_AddDestinationFailed{
			AddDestinationFailed: &pb.AddDestinationFailedEvent{
				Url:   evt.URL,
				Error: evt.Err.Error(),
			},
		},
	}
}

func buildDestinationStreamExitedEvent(evt event.DestinationStreamExitedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_DestinationStreamExited{
			DestinationStreamExited: &pb.DestinationStreamExitedEvent{
				Name:  evt.Name,
				Error: evt.Err.Error(),
			},
		},
	}
}

func buildStartDestinationFailedEvent(evt event.StartDestinationFailedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_StartDestinationFailed{
			StartDestinationFailed: &pb.StartDestinationFailedEvent{
				Id:      evt.ID[:],
				Message: evt.Message,
			},
		},
	}
}

func buildDestinationRemovedEvent(evt event.DestinationRemovedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_DestinationRemoved{
			DestinationRemoved: &pb.DestinationRemovedEvent{
				Id: evt.ID[:],
			},
		},
	}
}

func buildRemoveDestinationFailedEvent(evt event.RemoveDestinationFailedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_RemoveDestinationFailed{
			RemoveDestinationFailed: &pb.RemoveDestinationFailedEvent{
				Id:    evt.ID[:],
				Error: evt.Err.Error(),
			},
		},
	}
}

func buildFatalErrorOccurredEvent(evt event.FatalErrorOccurredEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_FatalError{
			FatalError: &pb.FatalErrorEvent{Message: evt.Message},
		},
	}
}

func buildOtherInstanceDetectedEvent(_ event.OtherInstanceDetectedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_OtherInstanceDetected{
			OtherInstanceDetected: &pb.OtherInstanceDetectedEvent{},
		},
	}
}

func buildMediaServerStartedEvent(event.MediaServerStartedEvent) *pb.Event {
	return &pb.Event{
		EventType: &pb.Event_MediaServerStarted{
			MediaServerStarted: &pb.MediaServerStartedEvent{},
		},
	}
}

// EventFromProto converts a protobuf message to an event.
func EventFromProto(pbEv *pb.Event) (event.Event, error) {
	if pbEv == nil || pbEv.EventType == nil {
		return nil, errors.New("nil or empty pb.Event")
	}

	switch evt := pbEv.EventType.(type) {
	case *pb.Event_AppStateChanged:
		return parseAppStateChangedEvent(evt.AppStateChanged)
	case *pb.Event_DestinationAdded:
		return parseDestinationAddedEvent(evt.DestinationAdded)
	case *pb.Event_AddDestinationFailed:
		return parseAddDestinationFailedEvent(evt.AddDestinationFailed)
	case *pb.Event_DestinationStreamExited:
		return parseDestinationStreamExitedEvent(evt.DestinationStreamExited)
	case *pb.Event_StartDestinationFailed:
		return parseStartDestinationFailedEvent(evt.StartDestinationFailed)
	case *pb.Event_DestinationRemoved:
		return parseDestinationRemovedEvent(evt.DestinationRemoved)
	case *pb.Event_RemoveDestinationFailed:
		return parseRemoveDestinationFailedEvent(evt.RemoveDestinationFailed)
	case *pb.Event_FatalError:
		return parseFatalErrorOccurredEvent(evt.FatalError)
	case *pb.Event_OtherInstanceDetected:
		return parseOtherInstanceDetectedEvent(evt.OtherInstanceDetected), nil
	case *pb.Event_MediaServerStarted:
		return parseMediaServerStartedEvent(evt.MediaServerStarted)
	default:
		return nil, fmt.Errorf("unknown event type: %T", evt)
	}
}

func parseAppStateChangedEvent(evt *pb.AppStateChangedEvent) (event.Event, error) {
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

func parseDestinationAddedEvent(evt *pb.DestinationAddedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationAddedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationAddedEvent{ID: id}, nil
}

func parseAddDestinationFailedEvent(evt *pb.AddDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil AddDestinationFailedEvent")
	}

	return event.AddDestinationFailedEvent{URL: evt.Url, Err: errors.New(evt.Error)}, nil
}

func parseDestinationStreamExitedEvent(evt *pb.DestinationStreamExitedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationStreamExitedEvent")
	}

	return event.DestinationStreamExitedEvent{Name: evt.Name, Err: errors.New(evt.Error)}, nil
}

func parseStartDestinationFailedEvent(evt *pb.StartDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil StartDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.StartDestinationFailedEvent{ID: id, Message: evt.Message}, nil
}

func parseDestinationRemovedEvent(evt *pb.DestinationRemovedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil DestinationRemovedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.DestinationRemovedEvent{ID: id}, nil
}

func parseRemoveDestinationFailedEvent(evt *pb.RemoveDestinationFailedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil RemoveDestinationFailedEvent")
	}

	id, err := uuid.FromBytes(evt.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.RemoveDestinationFailedEvent{ID: id, Err: errors.New(evt.Error)}, nil
}

func parseFatalErrorOccurredEvent(evt *pb.FatalErrorEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil FatalErrorEvent")
	}
	return event.FatalErrorOccurredEvent{Message: evt.Message}, nil
}

func parseOtherInstanceDetectedEvent(_ *pb.OtherInstanceDetectedEvent) event.Event {
	return event.OtherInstanceDetectedEvent{}
}

func parseMediaServerStartedEvent(evt *pb.MediaServerStartedEvent) (event.Event, error) {
	if evt == nil {
		return nil, errors.New("nil MediaServerStartedEvent")
	}
	return event.MediaServerStartedEvent{}, nil
}
