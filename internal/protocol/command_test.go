package protocol_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/optional"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/ptr"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCommandToProto(t *testing.T) {
	id := uuid.MustParse("0a193840-c24e-4c2f-93b5-eb3446088783")

	testCases := []struct {
		name string
		in   event.Command
		want *pb.Command
	}{
		{
			name: "AddDestination",
			in: event.CommandAddDestination{
				DestinationName: "test",
				URL:             "rtmp://rtmp.example.com",
			},
			want: &pb.Command{
				CommandType: &pb.Command_AddDestination{
					AddDestination: &pb.AddDestinationCommand{
						Name: "test",
						Url:  "rtmp://rtmp.example.com",
					},
				},
			},
		},
		{
			name: "UpdateDestination",
			in: event.CommandUpdateDestination{
				ID:              id,
				DestinationName: optional.New("test"),
				URL:             optional.New("rtmp://rtmp.example.com"),
			},
			want: &pb.Command{
				CommandType: &pb.Command_UpdateDestination{
					UpdateDestination: &pb.UpdateDestinationCommand{
						Id:   id[:],
						Name: ptr.New("test"),
						Url:  ptr.New("rtmp://rtmp.example.com"),
					},
				},
			},
		},
		{
			name: "UpdateDestination with partial fields",
			in: event.CommandUpdateDestination{
				ID:              id,
				DestinationName: optional.New("test"),
			},
			want: &pb.Command{
				CommandType: &pb.Command_UpdateDestination{
					UpdateDestination: &pb.UpdateDestinationCommand{
						Id:   id[:],
						Name: ptr.New("test"),
						Url:  nil,
					},
				},
			},
		},
		{
			name: "RemoveDestination",
			in:   event.CommandRemoveDestination{ID: id},
			want: &pb.Command{
				CommandType: &pb.Command_RemoveDestination{
					RemoveDestination: &pb.RemoveDestinationCommand{
						Id: id[:],
					},
				},
			},
		},
		{
			name: "StartDestination",
			in:   event.CommandStartDestination{ID: id},
			want: &pb.Command{
				CommandType: &pb.Command_StartDestination{
					StartDestination: &pb.StartDestinationCommand{Id: id[:]},
				},
			},
		},
		{
			name: "StopDestination",
			in:   event.CommandStopDestination{ID: id},
			want: &pb.Command{
				CommandType: &pb.Command_StopDestination{
					StopDestination: &pb.StopDestinationCommand{Id: id[:]},
				},
			},
		},
		{
			name: "CloseOtherInstance",
			in:   event.CommandCloseOtherInstance{},
			want: &pb.Command{
				CommandType: &pb.Command_CloseOtherInstances{
					CloseOtherInstances: &pb.CloseOtherInstancesCommand{},
				},
			},
		},
		{
			name: "KillServer",
			in:   event.CommandKillServer{},
			want: &pb.Command{
				CommandType: &pb.Command_KillServer{
					KillServer: &pb.KillServerCommand{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, gocmp.Diff(tc.want, protocol.CommandToWrappedProto(tc.in), protocmp.Transform()))
		})
	}
}

// TestUnwrappedCommandConversion tests the unwrapped command conversion (both to and from proto)
func TestUnwrappedCommandConversion(t *testing.T) {
	id := uuid.MustParse("5aefdbf5-95c6-418e-b63a-c95682861db1")

	t.Run("AddDestinationCommand", func(t *testing.T) {
		domainCmd := event.CommandAddDestination{
			DestinationName: "test",
			URL:             "rtmp://rtmp.example.com",
		}

		// Conversion to proto
		protoCmd := protocol.AddDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, "test", protoCmd.Name)
		assert.Equal(t, "rtmp://rtmp.example.com", protoCmd.Url)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromAddDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandAddDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("UpdateDestinationCommand", func(t *testing.T) {
		domainCmd := event.CommandUpdateDestination{
			ID:              id,
			DestinationName: optional.New("test"),
			URL:             optional.New("rtmp://rtmp.example.com"),
		}

		// Conversion to proto
		protoCmd := protocol.UpdateDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, id[:], protoCmd.Id)
		assert.Equal(t, "test", *protoCmd.Name)
		assert.Equal(t, "rtmp://rtmp.example.com", *protoCmd.Url)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromUpdateDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandUpdateDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("UpdateDestinationCommand with partial fields", func(t *testing.T) {
		domainCmd := event.CommandUpdateDestination{
			ID:              id,
			DestinationName: optional.New("test"),
			URL:             optional.Empty[string](),
		}

		// Conversion to proto
		protoCmd := protocol.UpdateDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, id[:], protoCmd.Id)
		assert.Equal(t, "test", *protoCmd.Name)
		assert.Nil(t, protoCmd.Url)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromUpdateDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandUpdateDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("RemoveDestinationCommand", func(t *testing.T) {
		domainCmd := event.CommandRemoveDestination{ID: id}

		// Conversion to proto
		protoCmd := protocol.RemoveDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, id[:], protoCmd.Id)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromRemoveDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandRemoveDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("StartDestinationCommand", func(t *testing.T) {
		domainCmd := event.CommandStartDestination{ID: id}

		// Conversion to proto
		protoCmd := protocol.StartDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, id[:], protoCmd.Id)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromStartDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandStartDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("StopDestinationCommand", func(t *testing.T) {
		domainCmd := event.CommandStopDestination{ID: id}

		// Conversion to proto
		protoCmd := protocol.StopDestinationCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)
		assert.Equal(t, id[:], protoCmd.Id)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromStopDestinationProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandStopDestination{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("CloseOtherInstancesCommand", func(t *testing.T) {
		domainCmd := event.CommandCloseOtherInstance{}

		// Conversion to proto
		protoCmd := protocol.CloseOtherInstancesCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromCloseOtherInstancesProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandCloseOtherInstance{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})

	t.Run("KillServerCommand", func(t *testing.T) {
		domainCmd := event.CommandKillServer{}

		// Conversion to proto
		protoCmd := protocol.KillServerCommandToProto(domainCmd)
		assert.NotNil(t, protoCmd)

		// Conversion from proto
		resultCmd, err := protocol.CommandFromKillServerProto(protoCmd)
		require.NoError(t, err)
		assert.IsType(t, event.CommandKillServer{}, resultCmd)
		assert.Empty(t, gocmp.Diff(domainCmd, resultCmd))
	})
}

func TestCommandFromProto(t *testing.T) {
	id := uuid.MustParse("5aefdbf5-95c6-418e-b63a-c95682861db1")

	testCases := []struct {
		name string
		in   *pb.Command
		want event.Command
	}{
		{
			name: "AddDestination",
			in: &pb.Command{
				CommandType: &pb.Command_AddDestination{
					AddDestination: &pb.AddDestinationCommand{
						Name: "test",
						Url:  "rtmp://rtmp.example.com",
					},
				},
			},
			want: event.CommandAddDestination{
				DestinationName: "test",
				URL:             "rtmp://rtmp.example.com",
			},
		},
		{
			name: "UpdateDestination",
			in: &pb.Command{
				CommandType: &pb.Command_UpdateDestination{
					UpdateDestination: &pb.UpdateDestinationCommand{
						Id:   id[:],
						Name: ptr.New("test"),
						Url:  ptr.New("rtmp://rtmp.example.com"),
					},
				},
			},
			want: event.CommandUpdateDestination{
				ID:              id,
				DestinationName: optional.New("test"),
				URL:             optional.New("rtmp://rtmp.example.com"),
			},
		},
		{
			name: "UpdateDestination with partial fields",
			in: &pb.Command{
				CommandType: &pb.Command_UpdateDestination{
					UpdateDestination: &pb.UpdateDestinationCommand{
						Id:   id[:],
						Name: ptr.New("test"),
					},
				},
			},
			want: event.CommandUpdateDestination{
				ID:              id,
				DestinationName: optional.New("test"),
				URL:             optional.Empty[string](),
			},
		},
		{
			name: "RemoveDestination",
			in: &pb.Command{
				CommandType: &pb.Command_RemoveDestination{
					RemoveDestination: &pb.RemoveDestinationCommand{Id: id[:]},
				},
			},
			want: event.CommandRemoveDestination{ID: id},
		},
		{
			name: "StartDestination",
			in: &pb.Command{
				CommandType: &pb.Command_StartDestination{
					StartDestination: &pb.StartDestinationCommand{Id: id[:]},
				},
			},
			want: event.CommandStartDestination{ID: id},
		},
		{
			name: "StopDestination",
			in: &pb.Command{
				CommandType: &pb.Command_StopDestination{
					StopDestination: &pb.StopDestinationCommand{Id: id[:]},
				},
			},
			want: event.CommandStopDestination{ID: id},
		},
		{
			name: "CloseOtherInstance",
			in: &pb.Command{
				CommandType: &pb.Command_CloseOtherInstances{
					CloseOtherInstances: &pb.CloseOtherInstancesCommand{},
				},
			},
			want: event.CommandCloseOtherInstance{},
		},
		{
			name: "KillServer",
			in: &pb.Command{
				CommandType: &pb.Command_KillServer{
					KillServer: &pb.KillServerCommand{},
				},
			},
			want: event.CommandKillServer{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got2, err2 := protocol.CommandFromWrappedProto(tc.in)
			require.NoError(t, err2)
			assert.Empty(t, gocmp.Diff(tc.want, got2))
		})
	}
}
