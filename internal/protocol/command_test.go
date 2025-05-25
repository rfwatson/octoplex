package protocol_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
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
			assert.Empty(t, gocmp.Diff(tc.want, protocol.CommandToProto(tc.in), protocmp.Transform()))
		})
	}
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
			got, err := protocol.CommandFromProto(tc.in)
			require.NoError(t, err)
			assert.Empty(t, gocmp.Diff(tc.want, got))
		})
	}
}
