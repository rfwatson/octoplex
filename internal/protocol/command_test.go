package protocol_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestCommandToProto(t *testing.T) {
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
			in: event.CommandRemoveDestination{
				URL: "rtmp://remove.example.com",
			},
			want: &pb.Command{
				CommandType: &pb.Command_RemoveDestination{
					RemoveDestination: &pb.RemoveDestinationCommand{
						Url: "rtmp://remove.example.com",
					},
				},
			},
		},
		{
			name: "StartDestination",
			in: event.CommandStartDestination{
				URL: "rtmp://start.example.com",
			},
			want: &pb.Command{
				CommandType: &pb.Command_StartDestination{
					StartDestination: &pb.StartDestinationCommand{
						Url: "rtmp://start.example.com",
					},
				},
			},
		},
		{
			name: "StopDestination",
			in: event.CommandStopDestination{
				URL: "rtmp://stop.example.com",
			},
			want: &pb.Command{
				CommandType: &pb.Command_StopDestination{
					StopDestination: &pb.StopDestinationCommand{
						Url: "rtmp://stop.example.com",
					},
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
					RemoveDestination: &pb.RemoveDestinationCommand{
						Url: "rtmp://remove.example.com",
					},
				},
			},
			want: event.CommandRemoveDestination{URL: "rtmp://remove.example.com"},
		},
		{
			name: "StartDestination",
			in: &pb.Command{
				CommandType: &pb.Command_StartDestination{
					StartDestination: &pb.StartDestinationCommand{
						Url: "rtmp://start.example.com",
					},
				},
			},
			want: event.CommandStartDestination{URL: "rtmp://start.example.com"},
		},
		{
			name: "StopDestination",
			in: &pb.Command{
				CommandType: &pb.Command_StopDestination{
					StopDestination: &pb.StopDestinationCommand{
						Url: "rtmp://stop.example.com",
					},
				},
			},
			want: event.CommandStopDestination{URL: "rtmp://stop.example.com"},
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
			assert.Empty(t, gocmp.Diff(tc.want, protocol.CommandFromProto(tc.in)))
		})
	}
}
