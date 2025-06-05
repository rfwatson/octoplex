package protocol

import (
	"fmt"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/optional"
	"github.com/google/uuid"
)

// CommandToWrappedProto converts a command to a wrapped protobuf message (inside pb.Command).
// For direct conversion to the specific command type, use the *CommandToProto functions.
func CommandToWrappedProto(command event.Command) *pb.Command {
	switch cmd := command.(type) {
	case event.CommandListDestinations:
		return &pb.Command{
			CommandType: &pb.Command_ListDestinations{
				ListDestinations: ListDestinationsCommandToProto(cmd),
			},
		}
	case event.CommandAddDestination:
		return &pb.Command{
			CommandType: &pb.Command_AddDestination{
				AddDestination: AddDestinationCommandToProto(cmd),
			},
		}
	case event.CommandUpdateDestination:
		return &pb.Command{
			CommandType: &pb.Command_UpdateDestination{
				UpdateDestination: UpdateDestinationCommandToProto(cmd),
			},
		}
	case event.CommandRemoveDestination:
		return &pb.Command{
			CommandType: &pb.Command_RemoveDestination{
				RemoveDestination: RemoveDestinationCommandToProto(cmd),
			},
		}
	case event.CommandStartDestination:
		return &pb.Command{
			CommandType: &pb.Command_StartDestination{
				StartDestination: StartDestinationCommandToProto(cmd),
			},
		}
	case event.CommandStopDestination:
		return &pb.Command{
			CommandType: &pb.Command_StopDestination{
				StopDestination: StopDestinationCommandToProto(cmd),
			},
		}
	case event.CommandCloseOtherInstance:
		return &pb.Command{
			CommandType: &pb.Command_CloseOtherInstances{
				CloseOtherInstances: CloseOtherInstancesCommandToProto(cmd),
			},
		}
	case event.CommandKillServer:
		return &pb.Command{
			CommandType: &pb.Command_KillServer{
				KillServer: KillServerCommandToProto(cmd),
			},
		}
	default:
		panic("unknown command type")
	}
}

// ListDestinationsCommandToProto converts a ListDestinationsCommand to a protobuf message.
func ListDestinationsCommandToProto(event.CommandListDestinations) *pb.ListDestinationsCommand {
	return &pb.ListDestinationsCommand{}
}

// AddDestinationCommandToProto converts an AddDestinationCommand to a protobuf message.
func AddDestinationCommandToProto(cmd event.CommandAddDestination) *pb.AddDestinationCommand {
	return &pb.AddDestinationCommand{
		Name: cmd.DestinationName,
		Url:  cmd.URL,
	}
}

// UpdateDestinationCommandToProto converts an UpdateDestinationCommand to a protobuf message.
func UpdateDestinationCommandToProto(cmd event.CommandUpdateDestination) *pb.UpdateDestinationCommand {
	return &pb.UpdateDestinationCommand{
		Id:   cmd.ID[:],
		Name: cmd.DestinationName.Ref(),
		Url:  cmd.URL.Ref(),
	}
}

// RemoveDestinationCommandToProto converts a RemoveDestinationCommand to a protobuf message.
func RemoveDestinationCommandToProto(cmd event.CommandRemoveDestination) *pb.RemoveDestinationCommand {
	return &pb.RemoveDestinationCommand{Id: cmd.ID[:], Force: cmd.Force}
}

// StartDestinationCommandToProto converts a StartDestinationCommand to a protobuf message.
func StartDestinationCommandToProto(cmd event.CommandStartDestination) *pb.StartDestinationCommand {
	return &pb.StartDestinationCommand{Id: cmd.ID[:]}
}

// StopDestinationCommandToProto converts a StopDestinationCommand to a protobuf message.
func StopDestinationCommandToProto(cmd event.CommandStopDestination) *pb.StopDestinationCommand {
	return &pb.StopDestinationCommand{Id: cmd.ID[:]}
}

// CloseOtherInstancesCommandToProto converts a CloseOtherInstanceCommand to a protobuf message.
func CloseOtherInstancesCommandToProto(event.CommandCloseOtherInstance) *pb.CloseOtherInstancesCommand {
	return &pb.CloseOtherInstancesCommand{}
}

// KillServerCommandToProto converts a KillServerCommand to a protobuf message.
func KillServerCommandToProto(event.CommandKillServer) *pb.KillServerCommand {
	return &pb.KillServerCommand{}
}

// CommandFromWrappedProto converts a wrapped protobuf message to a command.
// This is intended for processing commands from a streaming gRPC API where commands are represented in a oneof.
// For direct conversion from specific command types, use the CommandFrom*Proto functions.
func CommandFromWrappedProto(pbCmd *pb.Command) (event.Command, error) {
	if pbCmd == nil || pbCmd.CommandType == nil {
		return nil, fmt.Errorf("nil or empty Command")
	}

	switch cmd := pbCmd.CommandType.(type) {
	case *pb.Command_ListDestinations:
		return CommandFromListDestinationsProto(cmd.ListDestinations)
	case *pb.Command_AddDestination:
		return CommandFromAddDestinationProto(cmd.AddDestination)
	case *pb.Command_UpdateDestination:
		return CommandFromUpdateDestinationProto(cmd.UpdateDestination)
	case *pb.Command_RemoveDestination:
		return CommandFromRemoveDestinationProto(cmd.RemoveDestination)
	case *pb.Command_StartDestination:
		return CommandFromStartDestinationProto(cmd.StartDestination)
	case *pb.Command_StopDestination:
		return CommandFromStopDestinationProto(cmd.StopDestination)
	case *pb.Command_CloseOtherInstances:
		return CommandFromCloseOtherInstancesProto(cmd.CloseOtherInstances)
	case *pb.Command_KillServer:
		return CommandFromKillServerProto(cmd.KillServer)
	default:
		return nil, fmt.Errorf("unknown command type: %T", cmd)
	}
}

// CommandFromListDestinationsProto converts a protobuf ListDestinationsCommand to a domain command.
func CommandFromListDestinationsProto(_ *pb.ListDestinationsCommand) (event.Command, error) {
	return event.CommandListDestinations{}, nil
}

// CommandFromAddDestinationProto converts a protobuf AddDestinationCommand to a domain command.
func CommandFromAddDestinationProto(cmd *pb.AddDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil AddDestinationCommand")
	}

	return event.CommandAddDestination{DestinationName: cmd.Name, URL: cmd.Url}, nil
}

// CommandFromUpdateDestinationProto converts a protobuf UpdateDestinationCommand to a domain command.
func CommandFromUpdateDestinationProto(cmd *pb.UpdateDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil UpdateDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandUpdateDestination{
		ID:              id,
		DestinationName: optional.Deref(cmd.Name),
		URL:             optional.Deref(cmd.Url),
	}, nil
}

// CommandFromRemoveDestinationProto converts a protobuf RemoveDestinationCommand to a domain command.
func CommandFromRemoveDestinationProto(cmd *pb.RemoveDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil RemoveDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandRemoveDestination{ID: id, Force: cmd.Force}, nil
}

// CommandFromStartDestinationProto converts a protobuf StartDestinationCommand to a domain command.
func CommandFromStartDestinationProto(cmd *pb.StartDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil StartDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandStartDestination{ID: id}, nil
}

// CommandFromStopDestinationProto converts a protobuf StopDestinationCommand to a domain command.
func CommandFromStopDestinationProto(cmd *pb.StopDestinationCommand) (event.Command, error) {
	if cmd == nil {
		return nil, fmt.Errorf("nil StopDestinationCommand")
	}

	id, err := uuid.FromBytes(cmd.Id)
	if err != nil {
		return nil, fmt.Errorf("parse ID: %w", err)
	}

	return event.CommandStopDestination{ID: id}, nil
}

// CommandFromCloseOtherInstancesProto converts a protobuf CloseOtherInstancesCommand to a domain command.
func CommandFromCloseOtherInstancesProto(_ *pb.CloseOtherInstancesCommand) (event.Command, error) {
	return event.CommandCloseOtherInstance{}, nil
}

// CommandFromKillServerProto converts a protobuf KillServerCommand to a domain command.
func CommandFromKillServerProto(_ *pb.KillServerCommand) (event.Command, error) {
	return event.CommandKillServer{}, nil
}
