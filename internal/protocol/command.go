package protocol

import (
	"fmt"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"github.com/google/uuid"
)

// CommandToProto converts a command to a protobuf message.
func CommandToProto(command event.Command) *pb.Command {
	switch evt := command.(type) {
	case event.CommandAddDestination:
		return buildAddDestinationCommand(evt)
	case event.CommandUpdateDestination:
		return buildUpdateDestinationCommand(evt)
	case event.CommandRemoveDestination:
		return buildRemoveDestinationCommand(evt)
	case event.CommandStartDestination:
		return buildStartDestinationCommand(evt)
	case event.CommandStopDestination:
		return buildStopDestinationCommand(evt)
	case event.CommandCloseOtherInstance:
		return buildCloseOtherInstanceCommand(evt)
	case event.CommandKillServer:
		return buildKillServerCommand(evt)
	default:
		panic("unknown command type")
	}
}

func buildAddDestinationCommand(cmd event.CommandAddDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_AddDestination{AddDestination: &pb.AddDestinationCommand{Name: cmd.DestinationName, Url: cmd.URL}}}
}

func buildUpdateDestinationCommand(cmd event.CommandUpdateDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_UpdateDestination{UpdateDestination: &pb.UpdateDestinationCommand{Id: cmd.ID[:], Name: cmd.DestinationName, Url: cmd.URL}}}
}

func buildRemoveDestinationCommand(cmd event.CommandRemoveDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_RemoveDestination{RemoveDestination: &pb.RemoveDestinationCommand{Id: cmd.ID[:]}}}
}

func buildStartDestinationCommand(cmd event.CommandStartDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_StartDestination{StartDestination: &pb.StartDestinationCommand{Id: cmd.ID[:]}}}
}

func buildStopDestinationCommand(cmd event.CommandStopDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_StopDestination{StopDestination: &pb.StopDestinationCommand{Id: cmd.ID[:]}}}
}

func buildCloseOtherInstanceCommand(event.CommandCloseOtherInstance) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_CloseOtherInstances{CloseOtherInstances: &pb.CloseOtherInstancesCommand{}}}
}

func buildKillServerCommand(event.CommandKillServer) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_KillServer{KillServer: &pb.KillServerCommand{}}}
}

// CommandFromProto converts a protobuf message to a command.
func CommandFromProto(pbCmd *pb.Command) (event.Command, error) {
	if pbCmd == nil || pbCmd.CommandType == nil {
		return nil, fmt.Errorf("nil or empty Command")
	}

	switch cmd := pbCmd.CommandType.(type) {
	case *pb.Command_AddDestination:
		return parseAddDestinationCommand(cmd.AddDestination)
	case *pb.Command_UpdateDestination:
		return parseUpdateDestinationCommand(cmd.UpdateDestination)
	case *pb.Command_RemoveDestination:
		return parseRemoveDestinationCommand(cmd.RemoveDestination)
	case *pb.Command_StartDestination:
		return parseStartDestinationCommand(cmd.StartDestination)
	case *pb.Command_StopDestination:
		return parseStopDestinationCommand(cmd.StopDestination)
	case *pb.Command_CloseOtherInstances:
		return parseCloseOtherInstanceCommand(cmd.CloseOtherInstances), nil
	case *pb.Command_KillServer:
		return parseKillServerCommand(cmd.KillServer), nil
	default:
		return nil, fmt.Errorf("unknown command type: %T", cmd)
	}
}

func parseAddDestinationCommand(cmd *pb.AddDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil AddDestinationCommand")
	}

	return event.CommandAddDestination{DestinationName: cmd.Name, URL: cmd.Url}, nil
}

func parseUpdateDestinationCommand(cmd *pb.UpdateDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil UpdateDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandUpdateDestination{
		ID:              id,
		DestinationName: cmd.Name,
		URL:             cmd.Url,
	}, nil
}

func parseRemoveDestinationCommand(cmd *pb.RemoveDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil RemoveDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandRemoveDestination{ID: id}, nil
}

func parseStartDestinationCommand(cmd *pb.StartDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil StartDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandStartDestination{ID: id}, nil
}

func parseStopDestinationCommand(cmd *pb.StopDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil StopDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandStopDestination{ID: id}, nil
}

func parseCloseOtherInstanceCommand(_ *pb.CloseOtherInstancesCommand) event.Command {
	return event.CommandCloseOtherInstance{}
}

func parseKillServerCommand(_ *pb.KillServerCommand) event.Command {
	return event.CommandKillServer{}
}
