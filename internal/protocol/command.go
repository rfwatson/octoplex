package protocol

import (
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
)

// CommandToProto converts a command to a protobuf message.
func CommandToProto(command event.Command) *pb.Command {
	switch evt := command.(type) {
	case event.CommandAddDestination:
		return buildAddDestinationCommand(evt)
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

func buildRemoveDestinationCommand(cmd event.CommandRemoveDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_RemoveDestination{RemoveDestination: &pb.RemoveDestinationCommand{Url: cmd.URL}}}
}

func buildStartDestinationCommand(cmd event.CommandStartDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_StartDestination{StartDestination: &pb.StartDestinationCommand{Url: cmd.URL}}}
}

func buildStopDestinationCommand(cmd event.CommandStopDestination) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_StopDestination{StopDestination: &pb.StopDestinationCommand{Url: cmd.URL}}}
}

func buildCloseOtherInstanceCommand(event.CommandCloseOtherInstance) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_CloseOtherInstances{CloseOtherInstances: &pb.CloseOtherInstancesCommand{}}}
}

func buildKillServerCommand(event.CommandKillServer) *pb.Command {
	return &pb.Command{CommandType: &pb.Command_KillServer{KillServer: &pb.KillServerCommand{}}}
}

// CommandFromProto converts a protobuf message to a command.
func CommandFromProto(pbCmd *pb.Command) event.Command {
	if pbCmd == nil || pbCmd.CommandType == nil {
		panic("invalid or nil pb.Command")
	}

	switch cmd := pbCmd.CommandType.(type) {
	case *pb.Command_AddDestination:
		return parseAddDestinationCommand(cmd.AddDestination)
	case *pb.Command_RemoveDestination:
		return parseRemoveDestinationCommand(cmd.RemoveDestination)
	case *pb.Command_StartDestination:
		return parseStartDestinationCommand(cmd.StartDestination)
	case *pb.Command_StopDestination:
		return parseStopDestinationCommand(cmd.StopDestination)
	case *pb.Command_CloseOtherInstances:
		return parseCloseOtherInstanceCommand(cmd.CloseOtherInstances)
	case *pb.Command_KillServer:
		return parseKillServerCommand(cmd.KillServer)
	default:
		panic("unknown pb.Command type")
	}
}

func parseAddDestinationCommand(cmd *pb.AddDestinationCommand) event.Command {
	if cmd == nil {
		panic("nil AddDestinationCommand")
	}
	return event.CommandAddDestination{DestinationName: cmd.Name, URL: cmd.Url}
}

func parseRemoveDestinationCommand(cmd *pb.RemoveDestinationCommand) event.Command {
	if cmd == nil {
		panic("nil RemoveDestinationCommand")
	}
	return event.CommandRemoveDestination{URL: cmd.Url}
}

func parseStartDestinationCommand(cmd *pb.StartDestinationCommand) event.Command {
	if cmd == nil {
		panic("nil StartDestinationCommand")
	}
	return event.CommandStartDestination{URL: cmd.Url}
}

func parseStopDestinationCommand(cmd *pb.StopDestinationCommand) event.Command {
	if cmd == nil {
		panic("nil StopDestinationCommand")
	}
	return event.CommandStopDestination{URL: cmd.Url}
}

func parseCloseOtherInstanceCommand(_ *pb.CloseOtherInstancesCommand) event.Command {
	return event.CommandCloseOtherInstance{}
}

func parseKillServerCommand(_ *pb.KillServerCommand) event.Command {
	return event.CommandKillServer{}
}
