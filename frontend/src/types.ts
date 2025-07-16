// Frontend domain types.

export interface AppState {
  source: SourceState;
  destinations: Destination[];
}

export interface SourceState {
  live: boolean;
  liveChangedAt: Date | null;
  tracks: string[];
  rtmpURL: string;
  rtmpsURL: string;
  container: ContainerState;
}

export interface Destination {
  id: string;
  name: string;
  url: string;
  status: DestinationStatus;
  container: ContainerState;
}

export interface ContainerState {
  status: ContainerStatus;
  healthState: string;
  cpuPercent: number;
  memoryUsageBytes: number;
  rxRate: number;
  txRate: number;
  pullPercent: number;
  pullStatus: string;
  pullProgress: string;
  imageName: string;
  err: string | null;
}

export type DestinationStatus = 'live' | 'off-air' | 'starting';

export type ContainerStatus =
  | 'pulling'
  | 'created'
  | 'running'
  | 'restarting'
  | 'paused'
  | 'removing'
  | 'exited'
  | '';

// Event types for the application
export type AppEvent =
  | AppStateChangedEvent
  | DestinationAddedEvent
  | AddDestinationFailedEvent
  | DestinationUpdatedEvent
  | UpdateDestinationFailedEvent
  | DestinationRemovedEvent
  | DestinationStartedEvent
  | DestinationStoppedEvent
  | DestinationStreamExitedEvent
  | HandshakeCompletedEvent
  | ErrorEvent;

export interface AppStateChangedEvent {
  type: 'appStateChanged';
  state: AppState;
}

export interface DestinationAddedEvent {
  type: 'destinationAdded';
  id: string;
}

export interface AddDestinationFailedEvent {
  type: 'addDestinationFailed';
  url: string;
  error: string;
}

export interface DestinationUpdatedEvent {
  type: 'destinationUpdated';
  id: string;
}

export interface UpdateDestinationFailedEvent {
  type: 'updateDestinationFailed';
  id: string;
  error: string;
}

export interface DestinationRemovedEvent {
  type: 'destinationRemoved';
  id: string;
}

export interface DestinationStartedEvent {
  type: 'destinationStarted';
  id: string;
}

export interface DestinationStoppedEvent {
  type: 'destinationStopped';
  id: string;
}

export interface DestinationStreamExitedEvent {
  type: 'destinationStreamExited';
  id: string;
  name: string;
  error: string;
}

export interface HandshakeCompletedEvent {
  type: 'handshakeCompleted';
}

export interface ErrorEvent {
  type: 'error';
  message: string;
  details?: string;
}

// Command types for sending to backend
export type AppCommand =
  | AddDestinationCommand
  | UpdateDestinationCommand
  | RemoveDestinationCommand
  | StartDestinationCommand
  | StopDestinationCommand;

export interface AddDestinationCommand {
  type: 'addDestination';
  name: string;
  url: string;
}

export interface UpdateDestinationCommand {
  type: 'updateDestination';
  id: string;
  name?: string | undefined;
  url?: string | undefined;
}

export interface RemoveDestinationCommand {
  type: 'removeDestination';
  id: string;
  force: boolean;
}

export interface StartDestinationCommand {
  type: 'startDestination';
  id: string;
}

export interface StopDestinationCommand {
  type: 'stopDestination';
  id: string;
}
