// Event parser that converts protobuf events to domain types
import type { Event } from '../generated/internalapi/v1/event_pb.ts';
import type {
  AppEvent,
  AppState,
  SourceState,
  Destination,
  ContainerState,
} from './types.ts';

/**
 * Converts a bytes array (Uint8Array) to a string ID for easier use in the frontend
 */
function bytesToString(bytes: Uint8Array): string {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    '',
  );
}

/**
 * Converts protobuf destination status to our frontend enum
 */
function parseDestinationStatus(
  pbStatus: string | number,
): 'live' | 'off-air' | 'starting' {
  // Handle numeric enum values from protobuf
  if (typeof pbStatus === 'number') {
    switch (pbStatus) {
      case 0:
        return 'off-air'; // DestinationStatusOffAir
      case 1:
        return 'starting'; // DestinationStatusStarting
      case 2:
        return 'live'; // DestinationStatusLive
      default:
        return 'off-air';
    }
  }

  // Handle string values (fallback)
  switch (pbStatus) {
    case 'live':
      return 'live';
    case 'starting':
      return 'starting';
    case 'off-air':
    default:
      return 'off-air';
  }
}

/**
 * Converts protobuf container status to our frontend enum
 */
function parseContainerStatus(pbStatus: string): ContainerState['status'] {
  switch (pbStatus) {
    case 'pulling':
    case 'created':
    case 'running':
    case 'restarting':
    case 'paused':
    case 'removing':
    case 'exited':
      return pbStatus;
    default:
      return '';
  }
}

/**
 * Converts protobuf ContainerState to our frontend ContainerState
 */
function parseContainerState(pbContainer: any): ContainerState {
  return {
    status: parseContainerStatus(pbContainer?.status || ''),
    healthState: pbContainer?.healthState || '',
    cpuPercent: pbContainer?.cpuPercent || 0,
    memoryUsageBytes: pbContainer?.memoryUsageBytes || 0,
    rxRate: pbContainer?.rxRate || 0,
    txRate: pbContainer?.txRate || 0,
    pullPercent: pbContainer?.pullPercent || 0,
    pullStatus: pbContainer?.pullStatus || '',
    pullProgress: pbContainer?.pullProgress || '',
    imageName: pbContainer?.imageName || '',
    err: pbContainer?.err || null,
  };
}

/**
 * Converts protobuf SourceState to our frontend SourceState
 */
function parseSourceState(pbSource: any): SourceState {
  return {
    live: pbSource?.live || false,
    liveChangedAt: pbSource?.liveChangedAt
      ? new Date(pbSource.liveChangedAt)
      : null,
    tracks: pbSource?.tracks || [],
    rtmpURL: pbSource?.rtmpUrl || '',
    rtmpsURL: pbSource?.rtmpsUrl || '',
    container: parseContainerState(pbSource?.container),
  };
}

/**
 * Converts protobuf Destination to our frontend Destination
 */
function parseDestination(pbDestination: any): Destination {
  return {
    id: bytesToString(pbDestination?.id || new Uint8Array()),
    name: pbDestination?.name || '',
    url: pbDestination?.url || '',
    status: parseDestinationStatus(pbDestination?.status || ''),
    container: parseContainerState(pbDestination?.container),
  };
}

/**
 * Converts protobuf AppState to our frontend AppState
 */
function parseAppState(pbAppState: any): AppState {
  return {
    source: parseSourceState(pbAppState?.source),
    destinations: (pbAppState?.destinations || []).map(parseDestination),
  };
}

/**
 * Main event parser that converts protobuf Event to our frontend AppEvent
 */
export function parseEvent(pbEvent: Event): AppEvent | null {
  if (!pbEvent.eventType) {
    console.warn('Received event with no eventType');
    return null;
  }

  switch (pbEvent.eventType.case) {
    case 'appStateChanged':
      return {
        type: 'appStateChanged',
        state: parseAppState(pbEvent.eventType.value.appState),
      };

    case 'destinationAdded':
      return {
        type: 'destinationAdded',
        id: bytesToString(pbEvent.eventType.value.id),
      };

    case 'destinationUpdated':
      return {
        type: 'destinationUpdated',
        id: bytesToString(pbEvent.eventType.value.id),
      };

    case 'destinationRemoved':
      return {
        type: 'destinationRemoved',
        id: bytesToString(pbEvent.eventType.value.id),
      };

    case 'destinationStarted':
      return {
        type: 'destinationStarted',
        id: bytesToString(pbEvent.eventType.value.id),
      };

    case 'destinationStopped':
      return {
        type: 'destinationStopped',
        id: bytesToString(pbEvent.eventType.value.id),
      };

    case 'destinationStreamExited':
      return {
        type: 'destinationStreamExited',
        id: bytesToString(pbEvent.eventType.value.id),
        name: pbEvent.eventType.value.name,
        error: pbEvent.eventType.value.error,
      };

    case 'handshakeCompleted':
      return {
        type: 'handshakeCompleted',
      };

    case 'fatalError':
      return {
        type: 'error',
        message: pbEvent.eventType.value.message,
      };

    case 'addDestinationFailed':
      return {
        type: 'addDestinationFailed',
        url: pbEvent.eventType.value.url,
        error: pbEvent.eventType.value.error,
      };

    case 'updateDestinationFailed':
      return {
        type: 'updateDestinationFailed',
        id: bytesToString(pbEvent.eventType.value.id),
        error: pbEvent.eventType.value.error,
      };

    case 'removeDestinationFailed':
      return {
        type: 'error',
        message: `Failed to remove destination: ${pbEvent.eventType.value.error}`,
        details: `ID: ${bytesToString(pbEvent.eventType.value.id)}`,
      };

    case 'startDestinationFailed':
      return {
        type: 'error',
        message: `Failed to start destination: ${pbEvent.eventType.value.error}`,
        details: `ID: ${bytesToString(pbEvent.eventType.value.id)}`,
      };

    case 'stopDestinationFailed':
      return {
        type: 'error',
        message: `Failed to stop destination: ${pbEvent.eventType.value.error}`,
        details: `ID: ${bytesToString(pbEvent.eventType.value.id)}`,
      };

    default:
      console.log(`Unhandled event type: ${pbEvent.eventType.case}`);
      return null;
  }
}
