package protocol_test

import (
	"errors"
	"testing"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
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
			name: "DestinationsListed",
			in: event.DestinationsListedEvent{
				Destinations: []domain.Destination{
					{
						ID:   destinationID,
						Name: "dest1",
						URL:  "rtmp://dest1.example.com",
						Container: domain.Container{
							ID: "container1",
						},
					},
				},
			},
			want: &pb.Event{
				EventType: &pb.Event_DestinationsListed{
					DestinationsListed: &pb.DestinationsListedEvent{
						Destinations: []*pb.Destination{
							{
								Id:   destinationID[:],
								Name: "dest1",
								Url:  "rtmp://dest1.example.com",
								Container: &pb.Container{
									Id: "container1",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "ListDestinationsFailed",
			in:   event.ListDestinationsFailedEvent{Err: errors.New("failed to list destinations")},
			want: &pb.Event{
				EventType: &pb.Event_ListDestinationsFailed{
					ListDestinationsFailed: &pb.ListDestinationsFailedEvent{
						Error: "failed to list destinations",
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
			name: "DestinationRemoved",
			in:   event.DestinationRemovedEvent{ID: destinationID},
			want: &pb.Event{
				EventType: &pb.Event_DestinationRemoved{
					DestinationRemoved: &pb.DestinationRemovedEvent{Id: destinationID[:]},
				},
			},
		},
		{
			name: "RemoveDestinationFailed",
			in:   event.RemoveDestinationFailedEvent{ID: destinationID, Err: errors.New("removal failed")},
			want: &pb.Event{
				EventType: &pb.Event_RemoveDestinationFailed{
					RemoveDestinationFailed: &pb.RemoveDestinationFailedEvent{Id: destinationID[:], Error: "removal failed"},
				},
			},
		},
		{
			name: "DestinationStreamExited",
			in:   event.DestinationStreamExitedEvent{ID: destinationID, Name: "stream1", Err: errors.New("exit reason")},
			want: &pb.Event{
				EventType: &pb.Event_DestinationStreamExited{
					DestinationStreamExited: &pb.DestinationStreamExitedEvent{
						Id:    destinationID[:],
						Name:  "stream1",
						Error: "exit reason",
					},
				},
			},
		},
		{
			name: "DestinationStarted",
			in:   event.DestinationStartedEvent{ID: destinationID},
			want: &pb.Event{
				EventType: &pb.Event_DestinationStarted{
					DestinationStarted: &pb.DestinationStartedEvent{Id: destinationID[:]},
				},
			},
		},
		{
			name: "StartDestinationFailed",
			in:   event.StartDestinationFailedEvent{ID: destinationID, Err: errors.New("start failed")},
			want: &pb.Event{
				EventType: &pb.Event_StartDestinationFailed{
					StartDestinationFailed: &pb.StartDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "start failed",
					},
				},
			},
		},
		{
			name: "DestinationStopped",
			in:   event.DestinationStoppedEvent{ID: destinationID},
			want: &pb.Event{
				EventType: &pb.Event_DestinationStopped{
					DestinationStopped: &pb.DestinationStoppedEvent{Id: destinationID[:]},
				},
			},
		},
		{
			name: "StopDestinationFailed",
			in:   event.StopDestinationFailedEvent{ID: destinationID, Err: errors.New("stop failed")},
			want: &pb.Event{
				EventType: &pb.Event_StopDestinationFailed{
					StopDestinationFailed: &pb.StopDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "stop failed",
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
			assert.Empty(t, gocmp.Diff(tc.want, protocol.EventToWrappedProto(tc.in), protocmp.Transform()))
		})
	}
}

func TestUnwrappedEventConversion(t *testing.T) {
	destinationID := uuid.New()

	t.Run("AppStateChangedEvent", func(t *testing.T) {
		domainEvent := event.AppStateChangedEvent{
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
		}

		// Conversion to proto
		protoEvent := protocol.AppStateChangedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.NotNil(t, protoEvent.AppState)
		assert.NotNil(t, protoEvent.AppState.Source)
		assert.Equal(t, "abc123", protoEvent.AppState.Source.Container.Id)
		assert.True(t, protoEvent.AppState.Source.Live)
		assert.Equal(t, "rtmp://rtmp.example.com", protoEvent.AppState.Source.RtmpUrl)
		assert.Equal(t, "rtmps://rtmp.example.com", protoEvent.AppState.Source.RtmpsUrl)
		assert.Len(t, protoEvent.AppState.Destinations, 1)
		assert.Equal(t, destinationID[:], protoEvent.AppState.Destinations[0].Id)
		assert.Equal(t, "dest1", protoEvent.AppState.Destinations[0].Name)
		assert.Equal(t, "rtmp://dest1.example.com", protoEvent.AppState.Destinations[0].Url)
		assert.Equal(t, "bcd456", protoEvent.AppState.Destinations[0].Container.Id)
		assert.Equal(t, "go1.16", protoEvent.AppState.BuildInfo.GoVersion)
		assert.Equal(t, "v1.0.0", protoEvent.AppState.BuildInfo.Version)

		// Conversion from proto
		resultEvent, err := protocol.EventFromAppStateChangedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.AppStateChangedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("DestinationAddedEvent", func(t *testing.T) {
		domainEvent := event.DestinationAddedEvent{ID: destinationID}

		// Conversion to proto
		protoEvent := protocol.DestinationAddedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationAddedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationAddedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("AddDestinationFailedEvent", func(t *testing.T) {
		domainEvent := event.AddDestinationFailedEvent{
			URL: "rtmp://fail.example.com",
			Err: errors.New("failed"),
		}

		// Conversion to proto
		protoEvent := protocol.AddDestinationFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, "rtmp://fail.example.com", protoEvent.Url)
		assert.Equal(t, "failed", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromAddDestinationFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.AddDestinationFailedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("DestinationUpdatedEvent", func(t *testing.T) {
		domainEvent := event.DestinationUpdatedEvent{ID: destinationID}

		// Conversion to proto
		protoEvent := protocol.DestinationUpdatedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationUpdatedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationUpdatedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("UpdateDestinationFailedEvent", func(t *testing.T) {
		domainEvent := event.UpdateDestinationFailedEvent{
			ID:  destinationID,
			Err: errors.New("update failed"),
		}

		// Conversion to proto
		protoEvent := protocol.UpdateDestinationFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)
		assert.Equal(t, "update failed", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromUpdateDestinationFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.UpdateDestinationFailedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("DestinationStreamExitedEvent", func(t *testing.T) {
		domainEvent := event.DestinationStreamExitedEvent{
			ID:   destinationID,
			Name: "stream1",
			Err:  errors.New("exit reason"),
		}

		// Conversion to proto
		protoEvent := protocol.DestinationStreamExitedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, "stream1", protoEvent.Name)
		assert.Equal(t, "exit reason", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationStreamExitedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationStreamExitedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("FatalErrorOccurredEvent", func(t *testing.T) {
		domainEvent := event.FatalErrorOccurredEvent{Message: "fatal error"}

		// Conversion to proto
		protoEvent := protocol.FatalErrorEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, "fatal error", protoEvent.Message)

		// Conversion from proto
		resultEvent, err := protocol.EventFromFatalErrorProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.FatalErrorOccurredEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("DestinationStartedEvent", func(t *testing.T) {
		domainEvent := event.DestinationStartedEvent{ID: destinationID}

		// Conversion to proto
		protoEvent := protocol.DestinationStartedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationStartedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationStartedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("StartDestinationFailedEvent", func(t *testing.T) {
		domainEvent := event.StartDestinationFailedEvent{
			ID:  destinationID,
			Err: errors.New("start failed"),
		}

		// Conversion to proto
		protoEvent := protocol.StartDestinationFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)
		assert.Equal(t, "start failed", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromStartDestinationFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.StartDestinationFailedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("DestinationStoppedEvent", func(t *testing.T) {
		domainEvent := event.DestinationStoppedEvent{ID: destinationID}

		// Conversion to proto
		protoEvent := protocol.DestinationStoppedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationStoppedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationStoppedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("StopDestinationFailedEvent", func(t *testing.T) {
		domainEvent := event.StopDestinationFailedEvent{
			ID:  destinationID,
			Err: errors.New("stop failed"),
		}

		// Conversion to proto
		protoEvent := protocol.StopDestinationFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)
		assert.Equal(t, "stop failed", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromStopDestinationFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.StopDestinationFailedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("DestinationRemovedEvent", func(t *testing.T) {
		domainEvent := event.DestinationRemovedEvent{ID: destinationID}

		// Conversion to proto
		protoEvent := protocol.DestinationRemovedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationRemovedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationRemovedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("RemoveDestinationFailedEvent", func(t *testing.T) {
		domainEvent := event.RemoveDestinationFailedEvent{
			ID:  destinationID,
			Err: errors.New("removal failed"),
		}

		// Conversion to proto
		protoEvent := protocol.RemoveDestinationFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, destinationID[:], protoEvent.Id)
		assert.Equal(t, "removal failed", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromRemoveDestinationFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.RemoveDestinationFailedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent, gocmp.Comparer(compareErrorMessages)))
	})

	t.Run("OtherInstanceDetectedEvent", func(t *testing.T) {
		domainEvent := event.OtherInstanceDetectedEvent{}

		// Conversion to proto
		protoEvent := protocol.OtherInstanceDetectedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)

		// Conversion from proto
		resultEvent, err := protocol.EventFromOtherInstanceDetectedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.OtherInstanceDetectedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("MediaServerStartedEvent", func(t *testing.T) {
		domainEvent := event.MediaServerStartedEvent{}

		// Conversion to proto
		protoEvent := protocol.MediaServerStartedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)

		// Conversion from proto
		resultEvent, err := protocol.EventFromMediaServerStartedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.MediaServerStartedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("DestinationsListedEvent", func(t *testing.T) {
		domainEvent := event.DestinationsListedEvent{
			Destinations: []domain.Destination{
				{
					ID:   destinationID,
					Name: "dest1",
					URL:  "rtmp://dest1.example.com",
					Container: domain.Container{
						ID: "container1",
					},
				},
			},
		}

		// Conversion to proto
		protoEvent := protocol.DestinationsListedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Len(t, protoEvent.Destinations, 1)
		assert.Equal(t, destinationID[:], protoEvent.Destinations[0].Id)
		assert.Equal(t, "dest1", protoEvent.Destinations[0].Name)
		assert.Equal(t, "rtmp://dest1.example.com", protoEvent.Destinations[0].Url)

		// Conversion from proto
		resultEvent, err := protocol.EventFromDestinationsListedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.DestinationsListedEvent{}, resultEvent)
		assert.Empty(t, gocmp.Diff(domainEvent, resultEvent))
	})

	t.Run("ListDestinationsFailedEvent", func(t *testing.T) {
		domainEvent := event.ListDestinationsFailedEvent{
			Err: errors.New("failed to list destinations"),
		}

		// Conversion to proto
		protoEvent := protocol.ListDestinationsFailedEventToProto(domainEvent)
		assert.NotNil(t, protoEvent)
		assert.Equal(t, "failed to list destinations", protoEvent.Error)

		// Conversion from proto
		resultEvent, err := protocol.EventFromListDestinationsFailedProto(protoEvent)
		require.NoError(t, err)
		assert.IsType(t, event.ListDestinationsFailedEvent{}, resultEvent)
		assert.Equal(t, domainEvent.Err.Error(), resultEvent.(event.ListDestinationsFailedEvent).Err.Error())
	})
}

// TestEventFromProto tests the wrapped event conversion
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
			name: "DestinationsListed",
			in: &pb.Event{
				EventType: &pb.Event_DestinationsListed{
					DestinationsListed: &pb.DestinationsListedEvent{
						Destinations: []*pb.Destination{
							{
								Id:   destinationID[:],
								Name: "dest1",
								Url:  "rtmp://dest1.example.com",
								Container: &pb.Container{
									Id: "container1",
								},
							},
						},
					},
				},
			},
			want: event.DestinationsListedEvent{
				Destinations: []domain.Destination{
					{
						ID:   destinationID,
						Name: "dest1",
						URL:  "rtmp://dest1.example.com",
						Container: domain.Container{
							ID: "container1",
						},
					},
				},
			},
		},
		{
			name: "ListDestinationsFailed",
			in: &pb.Event{
				EventType: &pb.Event_ListDestinationsFailed{
					ListDestinationsFailed: &pb.ListDestinationsFailedEvent{
						Error: "failed to list destinations",
					},
				},
			},
			want: event.ListDestinationsFailedEvent{Err: errors.New("failed to list destinations")},
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
			name: "DestinationRemoved",
			in: &pb.Event{
				EventType: &pb.Event_DestinationRemoved{
					DestinationRemoved: &pb.DestinationRemovedEvent{
						Id: destinationID[:],
					},
				},
			},
			want: event.DestinationRemovedEvent{ID: destinationID},
		},
		{
			name: "RemoveDestinationFailed",
			in: &pb.Event{
				EventType: &pb.Event_RemoveDestinationFailed{
					RemoveDestinationFailed: &pb.RemoveDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "removal failed",
					},
				},
			},
			want: event.RemoveDestinationFailedEvent{ID: destinationID, Err: errors.New("removal failed")},
		},
		{
			name: "DestinationStreamExited",
			in: &pb.Event{
				EventType: &pb.Event_DestinationStreamExited{
					DestinationStreamExited: &pb.DestinationStreamExitedEvent{
						Id:    destinationID[:],
						Name:  "stream1",
						Error: "exit reason",
					},
				},
			},
			want: event.DestinationStreamExitedEvent{ID: destinationID, Name: "stream1", Err: errors.New("exit reason")},
		},
		{
			name: "DestinationStarted",
			in: &pb.Event{
				EventType: &pb.Event_DestinationStarted{
					DestinationStarted: &pb.DestinationStartedEvent{
						Id: destinationID[:],
					},
				},
			},
			want: event.DestinationStartedEvent{ID: destinationID},
		},
		{
			name: "StartDestinationFailed",
			in: &pb.Event{
				EventType: &pb.Event_StartDestinationFailed{
					StartDestinationFailed: &pb.StartDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "start failed",
					},
				},
			},
			want: event.StartDestinationFailedEvent{ID: destinationID, Err: errors.New("start failed")},
		},
		{
			name: "DestinationStopped",
			in: &pb.Event{
				EventType: &pb.Event_DestinationStopped{
					DestinationStopped: &pb.DestinationStoppedEvent{
						Id: destinationID[:],
					},
				},
			},
			want: event.DestinationStoppedEvent{ID: destinationID},
		},
		{
			name: "StopDestinationFailed",
			in: &pb.Event{
				EventType: &pb.Event_StopDestinationFailed{
					StopDestinationFailed: &pb.StopDestinationFailedEvent{
						Id:    destinationID[:],
						Error: "stop failed",
					},
				},
			},
			want: event.StopDestinationFailedEvent{ID: destinationID, Err: errors.New("stop failed")},
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
			got, err := protocol.EventFromWrappedProto(tc.in)
			require.NoError(t, err)
			assert.Empty(t, gocmp.Diff(tc.want, got, gocmp.Comparer(compareErrorMessages)))
		})
	}
}
