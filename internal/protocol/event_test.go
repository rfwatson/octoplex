package protocol_test

import (
	"errors"
	"testing"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestEventToProto(t *testing.T) {
	destinationID := uuid.New()

	testCases := []struct {
		name string
		in   event.Event
		want *pb.Event
	}{
		{
			name: "AppStateChanged",
			in: event.AppStateChangedEvent{
				State: domain.AppState{
					Source: domain.Source{
						Container: domain.Container{
							ID: "abc123",
						},
						Live:     true,
						RTMPURL:  "rtmp://rtmp.example.com",
						RTMPSURL: "rtmps://rtmp.example.com",
					},
					Destinations: []domain.Destination{
						{
							ID:   destinationID,
							Name: "dest1",
							URL:  "rtmp://dest1.example.com",
							Container: domain.Container{
								ID: "bcd456",
							},
						},
					},
					BuildInfo: domain.BuildInfo{GoVersion: "go1.16", Version: "v1.0.0"},
				},
			},
			want: &pb.Event{
				EventType: &pb.Event_AppStateChanged{
					AppStateChanged: &pb.AppStateChangedEvent{
						AppState: &pb.AppState{
							Source: &pb.Source{
								Container: &pb.Container{
									Id: "abc123",
								},
								Live:     true,
								RtmpUrl:  "rtmp://rtmp.example.com",
								RtmpsUrl: "rtmps://rtmp.example.com",
							},
							Destinations: []*pb.Destination{
								{
									Id:   destinationID[:],
									Name: "dest1",
									Url:  "rtmp://dest1.example.com",
									Container: &pb.Container{
										Id: "bcd456",
									},
								},
							},
							BuildInfo: &pb.BuildInfo{GoVersion: "go1.16", Version: "v1.0.0"},
						},
					},
				},
			},
		},
		{
			name: "DestinationAdded",
			in:   event.DestinationAddedEvent{ID: destinationID},
			want: &pb.Event{
				EventType: &pb.Event_DestinationAdded{
					DestinationAdded: &pb.DestinationAddedEvent{Id: destinationID[:]},
				},
			},
		},
		{
			name: "AddDestinationFailed",
			in: event.AddDestinationFailedEvent{
				URL: "rtmp://fail.example.com",
				Err: errors.New("failed"),
			},
			want: &pb.Event{
				EventType: &pb.Event_AddDestinationFailed{
					AddDestinationFailed: &pb.AddDestinationFailedEvent{
						Url:   "rtmp://fail.example.com",
						Error: "failed",
					},
				},
			},
		},
		{
			name: "DestinationUpdated",
			in:   event.DestinationUpdatedEvent{ID: destinationID},
			want: &pb.Event{
				EventType: &pb.Event_DestinationUpdated{
					DestinationUpdated: &pb.DestinationUpdatedEvent{Id: destinationID[:]},
				},
			},
		},
		{
			name: "UpdateDestinationFailed",
			in:   event.UpdateDestinationFailedEvent{ID: destinationID, Err: errors.New("update failed")},
			want: &pb.Event{
				EventType: &pb.Event_UpdateDestinationFailed{
					UpdateDestinationFailed: &pb.UpdateDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "update failed",
					},
				},
			},
		},
		{
			name: "DestinationStreamExited",
			in:   event.DestinationStreamExitedEvent{Name: "stream1", Err: errors.New("exit reason")},
			want: &pb.Event{
				EventType: &pb.Event_DestinationStreamExited{
					DestinationStreamExited: &pb.DestinationStreamExitedEvent{
						Name:  "stream1",
						Error: "exit reason",
					},
				},
			},
		},
		{
			name: "FatalErrorOccurred",
			in:   event.FatalErrorOccurredEvent{Message: "fatal error"},
			want: &pb.Event{
				EventType: &pb.Event_FatalError{
					FatalError: &pb.FatalErrorEvent{Message: "fatal error"},
				},
			},
		},
		{
			name: "OtherInstanceDetected",
			in:   event.OtherInstanceDetectedEvent{},
			want: &pb.Event{
				EventType: &pb.Event_OtherInstanceDetected{
					OtherInstanceDetected: &pb.OtherInstanceDetectedEvent{},
				},
			},
		},
		{
			name: "MediaServerStarted",
			in:   event.MediaServerStartedEvent{},
			want: &pb.Event{
				EventType: &pb.Event_MediaServerStarted{
					MediaServerStarted: &pb.MediaServerStartedEvent{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, gocmp.Diff(tc.want, protocol.EventToProto(tc.in), protocmp.Transform()))
		})
	}
}

func TestEventFromProto(t *testing.T) {
	destinationID := uuid.New()

	testCases := []struct {
		name string
		in   *pb.Event
		want event.Event
	}{
		{
			name: "AppStateChanged",
			in: &pb.Event{
				EventType: &pb.Event_AppStateChanged{
					AppStateChanged: &pb.AppStateChangedEvent{
						AppState: &pb.AppState{
							Source: &pb.Source{
								Container: &pb.Container{Id: "abc123"},
								Live:      true,
								RtmpUrl:   "rtmp://rtmp.example.com",
								RtmpsUrl:  "rtmps://rtmp.example.com",
							},
							Destinations: []*pb.Destination{
								{
									Id:        destinationID[:],
									Name:      "dest1",
									Url:       "rtmp://dest1.example.com",
									Container: &pb.Container{Id: "bcd456"},
								},
							},
							BuildInfo: &pb.BuildInfo{
								GoVersion: "go1.16",
								Version:   "v1.0.0",
							},
						},
					},
				},
			},
			want: event.AppStateChangedEvent{
				State: domain.AppState{
					Source: domain.Source{
						Container: domain.Container{ID: "abc123"},
						Live:      true,
						RTMPURL:   "rtmp://rtmp.example.com",
						RTMPSURL:  "rtmps://rtmp.example.com",
					},
					Destinations: []domain.Destination{
						{
							ID:        destinationID,
							Name:      "dest1",
							URL:       "rtmp://dest1.example.com",
							Container: domain.Container{ID: "bcd456"},
						},
					},
					BuildInfo: domain.BuildInfo{
						GoVersion: "go1.16",
						Version:   "v1.0.0",
					},
				},
			},
		},
		{
			name: "DestinationAdded",
			in: &pb.Event{
				EventType: &pb.Event_DestinationAdded{
					DestinationAdded: &pb.DestinationAddedEvent{
						Id: destinationID[:],
					},
				},
			},
			want: event.DestinationAddedEvent{ID: destinationID},
		},
		{
			name: "AddDestinationFailed",
			in: &pb.Event{
				EventType: &pb.Event_AddDestinationFailed{
					AddDestinationFailed: &pb.AddDestinationFailedEvent{
						Url:   "rtmp://fail.example.com",
						Error: "failed",
					},
				},
			},
			want: event.AddDestinationFailedEvent{
				URL: "rtmp://fail.example.com",
				Err: errors.New("failed"),
			},
		},
		{
			name: "DestinationUpdated",
			in: &pb.Event{
				EventType: &pb.Event_DestinationUpdated{
					DestinationUpdated: &pb.DestinationUpdatedEvent{
						Id: destinationID[:],
					},
				},
			},
			want: event.DestinationUpdatedEvent{ID: destinationID},
		},
		{
			name: "UpdateDestinationFailed",
			in: &pb.Event{
				EventType: &pb.Event_UpdateDestinationFailed{
					UpdateDestinationFailed: &pb.UpdateDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "failed",
					},
				},
			},
			want: event.UpdateDestinationFailedEvent{
				ID:  destinationID,
				Err: errors.New("failed"),
			},
		},
		{
			name: "DestinationStreamExited",
			in: &pb.Event{
				EventType: &pb.Event_DestinationStreamExited{
					DestinationStreamExited: &pb.DestinationStreamExitedEvent{
						Name:  "stream1",
						Error: "exit reason",
					},
				},
			},
			want: event.DestinationStreamExitedEvent{Name: "stream1", Err: errors.New("exit reason")},
		},
		{
			name: "FatalErrorOccurred",
			in: &pb.Event{
				EventType: &pb.Event_FatalError{
					FatalError: &pb.FatalErrorEvent{Message: "fatal error"},
				},
			},
			want: event.FatalErrorOccurredEvent{Message: "fatal error"},
		},
		{
			name: "OtherInstanceDetected",
			in: &pb.Event{
				EventType: &pb.Event_OtherInstanceDetected{
					OtherInstanceDetected: &pb.OtherInstanceDetectedEvent{},
				},
			},
			want: event.OtherInstanceDetectedEvent{},
		},
		{
			name: "MediaServerStarted",
			in: &pb.Event{
				EventType: &pb.Event_MediaServerStarted{
					MediaServerStarted: &pb.MediaServerStartedEvent{},
				},
			},
			want: event.MediaServerStartedEvent{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := protocol.EventFromProto(tc.in)
			require.NoError(t, err)
			assert.Empty(t, gocmp.Diff(tc.want, got, gocmp.Comparer(compareErrorMessages)))
		})
	}
}
