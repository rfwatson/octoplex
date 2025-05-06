package protocol_test

import (
	"errors"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestContainerToProto(t *testing.T) {
	exitCode := 1
	ts := time.Unix(1234567890, 0)

	testCases := []struct {
		name string
		in   domain.Container
		want *pb.Container
	}{
		{
			name: "complete",
			in: domain.Container{
				ID:               "abc123",
				Status:           "running",
				HealthState:      "healthy",
				CPUPercent:       12.5,
				MemoryUsageBytes: 2048,
				RxRate:           100,
				TxRate:           200,
				RxSince:          ts,
				ImageName:        "nginx",
				PullStatus:       "pulling",
				PullProgress:     "50%",
				PullPercent:      50,
				RestartCount:     3,
				ExitCode:         &exitCode,
				Err:              errors.New("container error"),
			},
			want: &pb.Container{
				Id:               "abc123",
				Status:           "running",
				HealthState:      "healthy",
				CpuPercent:       12.5,
				MemoryUsageBytes: 2048,
				RxRate:           100,
				TxRate:           200,
				RxSince:          timestamppb.New(ts),
				ImageName:        "nginx",
				PullStatus:       "pulling",
				PullProgress:     "50%",
				PullPercent:      50,
				RestartCount:     3,
				ExitCode:         protoInt32(1),
				Err:              "container error",
			},
		},
		{
			name: "zero values",
			in:   domain.Container{},
			want: &pb.Container{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, gocmp.Diff(tc.want, protocol.ContainerToProto(tc.in), protocmp.Transform()))
		})
	}
}

func TestContainerFromProto(t *testing.T) {
	ts := timestamppb.New(time.Now())
	exitCode := int32(2)

	testCases := []struct {
		name string
		in   *pb.Container
		want domain.Container
	}{
		{
			name: "complete",
			in: &pb.Container{
				Id:               "xyz789",
				Status:           "exited",
				HealthState:      "unhealthy",
				CpuPercent:       42.0,
				MemoryUsageBytes: 4096,
				RxRate:           300,
				TxRate:           400,
				RxSince:          ts,
				ImageName:        "redis",
				PullStatus:       "complete",
				PullProgress:     "100%",
				PullPercent:      100,
				RestartCount:     1,
				ExitCode:         &exitCode,
				Err:              "crash error",
			},
			want: domain.Container{
				ID:               "xyz789",
				Status:           "exited",
				HealthState:      "unhealthy",
				CPUPercent:       42.0,
				MemoryUsageBytes: 4096,
				RxRate:           300,
				TxRate:           400,
				RxSince:          ts.AsTime(),
				ImageName:        "redis",
				PullStatus:       "complete",
				PullProgress:     "100%",
				PullPercent:      100,
				RestartCount:     1,
				ExitCode:         protoInt(2),
				Err:              errors.New("crash error"),
			},
		},
		{
			name: "nil proto",
			in:   nil,
			want: domain.Container{},
		},
		{
			name: "zero values",
			in:   &pb.Container{},
			want: domain.Container{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := protocol.ContainerFromProto(tc.in)
			assert.Empty(
				t,
				gocmp.Diff(
					tc.want,
					got,
					gocmp.Comparer(compareErrorMessages),
				))
		})
	}
}

func TestDestinationConversions(t *testing.T) {
	testCases := []struct {
		name string
		in   domain.Destination
		want *pb.Destination
	}{
		{
			name: "basic destination",
			in: domain.Destination{
				Name:      "dest1",
				URL:       "rtmp://dest1",
				Status:    domain.DestinationStatusLive,
				Container: domain.Container{ID: "c1"},
			},
			want: &pb.Destination{
				Name:      "dest1",
				Url:       "rtmp://dest1",
				Status:    pb.Destination_STATUS_LIVE,
				Container: &pb.Container{Id: "c1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proto := protocol.DestinationToProto(tc.in)
			assert.Equal(t, tc.want.Name, proto.Name)
			assert.Equal(t, tc.want.Url, proto.Url)
			assert.Equal(t, tc.want.Status, proto.Status)
			require.NotNil(t, proto.Container)
			assert.Equal(t, tc.want.Container.Id, proto.Container.Id)

			dests := protocol.ProtoToDestinations([]*pb.Destination{proto})
			assert.Len(t, dests, 1)
			assert.Equal(t, tc.in.Name, dests[0].Name)
			assert.Equal(t, tc.in.URL, dests[0].URL)
			assert.Equal(t, tc.in.Status, dests[0].Status)
			assert.Equal(t, tc.in.Container.ID, dests[0].Container.ID)
		})
	}
}

func TestDestinationStatusToProto(t *testing.T) {
	testCases := []struct {
		name string
		in   domain.DestinationStatus
		want pb.Destination_Status
	}{
		{"Starting", domain.DestinationStatusStarting, pb.Destination_STATUS_STARTING},
		{"Live", domain.DestinationStatusLive, pb.Destination_STATUS_LIVE},
		{"Off-air", domain.DestinationStatusOffAir, pb.Destination_STATUS_OFF_AIR},
		{"Unknown", domain.DestinationStatus(999), pb.Destination_STATUS_OFF_AIR},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, protocol.DestinationStatusToProto(tc.in))
		})
	}
}

func protoInt32(v int32) *int32 { return &v }
func protoInt(v int) *int       { return &v }

// compareErrorMessages compares two error messages for equality using only the
// error message string.
func compareErrorMessages(x, y error) bool {
	if x == nil || y == nil {
		return x == y
	}

	return x.Error() == y.Error()
}
