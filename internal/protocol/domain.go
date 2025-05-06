package protocol

import (
	"errors"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ContainerToProto converts a domain.Container to a protobuf Container.
func ContainerToProto(c domain.Container) *pb.Container {
	var errString string
	if c.Err != nil {
		errString = c.Err.Error()
	}

	var exitCode *int32
	if c.ExitCode != nil {
		code := int32(*c.ExitCode)
		exitCode = &code
	}

	var rxSince *timestamppb.Timestamp
	if !c.RxSince.IsZero() {
		rxSince = timestamppb.New(c.RxSince)
	}

	return &pb.Container{
		Id:               c.ID,
		Status:           c.Status,
		HealthState:      c.HealthState,
		CpuPercent:       c.CPUPercent,
		MemoryUsageBytes: c.MemoryUsageBytes,
		RxRate:           int32(c.RxRate),
		TxRate:           int32(c.TxRate),
		RxSince:          rxSince,
		ImageName:        c.ImageName,
		PullStatus:       c.PullStatus,
		PullProgress:     c.PullProgress,
		PullPercent:      int32(c.PullPercent),
		RestartCount:     int32(c.RestartCount),
		ExitCode:         exitCode,
		Err:              errString,
	}
}

// ContainerFromProto converts a protobuf Container to a domain.Container.
func ContainerFromProto(pbContainer *pb.Container) domain.Container {
	if pbContainer == nil {
		return domain.Container{}
	}

	var exitCode *int
	if pbContainer.ExitCode != nil {
		val := int(*pbContainer.ExitCode)
		exitCode = &val
	}

	var err error
	if pbContainer.Err != "" {
		err = errors.New(pbContainer.Err)
	}

	var rxSince time.Time
	if pbContainer.RxSince != nil {
		rxSince = pbContainer.RxSince.AsTime()
	}

	return domain.Container{
		ID:               pbContainer.Id,
		Status:           pbContainer.Status,
		HealthState:      pbContainer.HealthState,
		CPUPercent:       pbContainer.CpuPercent,
		MemoryUsageBytes: pbContainer.MemoryUsageBytes,
		RxRate:           int(pbContainer.RxRate),
		TxRate:           int(pbContainer.TxRate),
		RxSince:          rxSince,
		ImageName:        pbContainer.ImageName,
		PullStatus:       pbContainer.PullStatus,
		PullProgress:     pbContainer.PullProgress,
		PullPercent:      int(pbContainer.PullPercent),
		RestartCount:     int(pbContainer.RestartCount),
		ExitCode:         exitCode,
		Err:              err,
	}
}

// DestinationsToProto converts a slice of domain.Destinations to a slice of
// protobuf Destinations.
func DestinationsToProto(inDests []domain.Destination) []*pb.Destination {
	destinations := make([]*pb.Destination, 0, len(inDests))
	for _, d := range inDests {
		destinations = append(destinations, DestinationToProto(d))
	}
	return destinations
}

// DestinationToProto converts a domain.Destination to a protobuf Destination.
func DestinationToProto(d domain.Destination) *pb.Destination {
	return &pb.Destination{
		Container: ContainerToProto(d.Container),
		Status:    DestinationStatusToProto(d.Status),
		Name:      d.Name,
		Url:       d.URL,
	}
}

// ProtoToDestinations converts a slice of protobuf Destinations to a slice of
// domain.Destinations.
func ProtoToDestinations(pbDests []*pb.Destination) []domain.Destination {
	if pbDests == nil {
		return nil
	}

	dests := make([]domain.Destination, 0, len(pbDests))
	for _, pbDest := range pbDests {
		if pbDest == nil {
			continue
		}
		dests = append(dests, domain.Destination{
			Container: ContainerFromProto(pbDest.Container),
			Status:    domain.DestinationStatus(pbDest.Status),
			Name:      pbDest.Name,
			URL:       pbDest.Url,
		})
	}
	return dests
}

// DestinationStatusToProto converts a domain.DestinationStatus to a
// pb.Destination_Status.
func DestinationStatusToProto(s domain.DestinationStatus) pb.Destination_Status {
	switch s {
	case domain.DestinationStatusStarting:
		return pb.Destination_STATUS_STARTING
	case domain.DestinationStatusLive:
		return pb.Destination_STATUS_LIVE
	default:
		return pb.Destination_STATUS_OFF_AIR
	}
}
