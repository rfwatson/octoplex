// Command builder that converts domain commands to protobuf commands
import { create } from '@bufbuild/protobuf';
import type { Command } from '../generated/internalapi/v1/command_pb.ts';
import {
  CommandSchema,
  AddDestinationCommandSchema,
  UpdateDestinationCommandSchema,
  RemoveDestinationCommandSchema,
  StartDestinationCommandSchema,
  StopDestinationCommandSchema,
  StartHandshakeCommandSchema,
} from '../generated/internalapi/v1/command_pb.ts';
import type { AppCommand } from './types.ts';

/**
 * Converts a string ID to bytes array (Uint8Array) for protobuf.
 */
function stringToBytes(id: string): Uint8Array {
  if (id.length % 2 !== 0) {
    throw new Error(`Invalid ID format: ${id}`);
  }

  const bytes = new Uint8Array(id.length / 2);
  for (let i = 0; i < id.length; i += 2) {
    bytes[i / 2] = parseInt(id.substr(i, 2), 16);
  }
  return bytes;
}

/**
 * Builds a protobuf Command from domain type.
 */
export function buildCommand(command: AppCommand): Command {
  switch (command.type) {
    case 'addDestination':
      return create(CommandSchema, {
        commandType: {
          case: 'addDestination',
          value: create(AddDestinationCommandSchema, {
            name: command.name,
            url: command.url,
          }),
        },
      });

    case 'updateDestination':
      return create(CommandSchema, {
        commandType: {
          case: 'updateDestination',
          value: create(UpdateDestinationCommandSchema, {
            id: stringToBytes(command.id),
            ...(command.name !== undefined && { name: command.name }),
            ...(command.url !== undefined && { url: command.url }),
          }),
        },
      });

    case 'removeDestination':
      return create(CommandSchema, {
        commandType: {
          case: 'removeDestination',
          value: create(RemoveDestinationCommandSchema, {
            id: stringToBytes(command.id),
            force: command.force,
          }),
        },
      });

    case 'startDestination':
      return create(CommandSchema, {
        commandType: {
          case: 'startDestination',
          value: create(StartDestinationCommandSchema, {
            id: stringToBytes(command.id),
          }),
        },
      });

    case 'stopDestination':
      return create(CommandSchema, {
        commandType: {
          case: 'stopDestination',
          value: create(StopDestinationCommandSchema, {
            id: stringToBytes(command.id),
          }),
        },
      });

    default:
      throw new Error(`Unknown command type: ${JSON.stringify(command)}`);
  }
}

/**
 * Creates a StartHandshake command.
 */
export function buildStartHandshakeCommand(): Command {
  return create(CommandSchema, {
    commandType: {
      case: 'startHandshake',
      value: create(StartHandshakeCommandSchema, {}),
    },
  });
}
