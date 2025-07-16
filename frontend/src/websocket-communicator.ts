// WebSocket-based communicator that enables bidirectional streaming in browsers
import { create, fromBinary, toBinary } from '@bufbuild/protobuf';
import { EnvelopeSchema } from '../generated/internalapi/v1/api_pb.ts';
import type { Envelope } from '../generated/internalapi/v1/api_pb.ts';
import type { AppEvent, AppCommand } from './types.ts';
import { parseEvent } from './event-parser.ts';
import { buildCommand, buildStartHandshakeCommand } from './command-builder.ts';
import { joinUrl } from './helpers.ts';

export interface WebSocketCommunicatorOptions {
  baseUrl?: string;
  onEvent: (event: AppEvent) => void;
  onError: (error: Error) => void;
  onUnauthorized: () => void;
  onConnectionStateChange: (connected: boolean) => void;
}

export class WebSocketCommunicator {
  private ws: WebSocket | null = null;
  private url: string;
  private listeners: Set<(envelope: Envelope) => void> = new Set();
  private reconnectAttempts = 0;
  private readonly maxReconnectAttempts = 5;
  private readonly reconnectDelay = 1000;
  private isConnected = false;
  private commandQueue: AppCommand[] = [];

  constructor(private options: WebSocketCommunicatorOptions) {
    if (import.meta.env.DEV) {
      this.url = joinUrl(window.location.origin.replace(/^http/, 'ws'), 'ws');
    } else {
      const baseUrl = options.baseUrl || window.location.origin;
      this.url = joinUrl(baseUrl.replace(/^http/, 'ws'), 'ws');
    }
  }

  async connect(): Promise<void> {
    if (this.isConnected) {
      console.warn('Already connected to WebSocket');
      return;
    }

    try {
      this.initWebSocket();
    } catch (error) {
      console.error('Failed to connect to WebSocket:', error);
      this.options.onError(
        error instanceof Error ? error : new Error(String(error)),
      );
      this.scheduleReconnect();
    }
  }

  /**
   * Disconnect from WebSocket
   */
  disconnect(): void {
    this.isConnected = false;
    this.reconnectAttempts = 0;

    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    this.options.onConnectionStateChange(false);
  }

  /**
   * Send a command to the backend
   */
  async sendCommand(command: AppCommand): Promise<void> {
    if (!this.isConnected) {
      // Queue the command for when connection is established
      this.commandQueue.push(command);
      return;
    }

    try {
      const envelope = create(EnvelopeSchema, {
        payload: {
          case: 'command',
          value: buildCommand(command),
        },
      });

      this.sendEnvelope(envelope);
    } catch (error) {
      console.error('Failed to send command:', error);
      this.options.onError(
        error instanceof Error ? error : new Error(String(error)),
      );
    }
  }

  /**
   * Initialize WebSocket connection
   */
  private initWebSocket(): void {
    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      console.log('WebSocket connected to', this.url);
      this.isConnected = true;
      this.reconnectAttempts = 0;
      this.options.onConnectionStateChange(true);

      // Send handshake immediately
      this.sendHandshake();

      // Send any queued commands
      this.processCommandQueue();
    };

    this.ws.onmessage = async (event) => {
      try {
        const arrayBuffer = await event.data.arrayBuffer();
        const bytes = new Uint8Array(arrayBuffer);
        const envelope = fromBinary(EnvelopeSchema, bytes);
        this.handleEnvelope(envelope);
      } catch (error) {
        console.error('Failed to parse WebSocket message:', error);
        this.options.onError(new Error('Invalid message format'));
      }
    };

    this.ws.onclose = (event) => {
      this.isConnected = false;
      this.options.onConnectionStateChange(false);

      if (event.code === 4003) {
        // Unauthorized
        this.options.onUnauthorized();
        return;
      }

      // Only attempt to reconnect if it wasn't a clean close
      if (
        event.code !== 1000 &&
        this.reconnectAttempts < this.maxReconnectAttempts
      ) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      this.options.onError(new Error('WebSocket connection error'));
    };
  }

  /**
   * Send handshake command to initiate communication
   */
  private sendHandshake(): void {
    const envelope = create(EnvelopeSchema, {
      payload: {
        case: 'command',
        value: buildStartHandshakeCommand(),
      },
    });

    this.sendEnvelope(envelope);
  }

  /**
   * Process any queued commands
   */
  private processCommandQueue(): void {
    while (this.commandQueue.length > 0) {
      const command = this.commandQueue.shift()!;
      this.sendCommand(command);
    }
  }

  /**
   * Send an envelope over WebSocket
   */
  private sendEnvelope(envelope: Envelope): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      // Send binary protobuf data
      const bytes = toBinary(EnvelopeSchema, envelope);
      this.ws.send(bytes);
    } else {
      console.warn('WebSocket not ready, message dropped');
    }
  }

  /**
   * Handle incoming envelope and convert to AppEvent
   */
  private handleEnvelope(envelope: Envelope): void {
    if (envelope.payload?.case === 'event') {
      const event = parseEvent(envelope.payload.value);
      if (event) {
        this.options.onEvent(event);
      }
    }

    // Also notify any direct envelope listeners
    this.listeners.forEach((listener) => listener(envelope));
  }

  /**
   * Add a direct envelope listener (for compatibility)
   */
  onEvent(handler: (envelope: Envelope) => void): () => void {
    this.listeners.add(handler);
    return () => this.listeners.delete(handler);
  }

  /**
   * Send a raw envelope (for compatibility)
   */
  sendRawCommand(envelope: Envelope): void {
    this.sendEnvelope(envelope);
  }

  /**
   * Schedule a reconnection attempt with exponential backoff
   */
  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max WebSocket reconnection attempts reached');
      this.options.onError(
        new Error('Failed to reconnect after maximum attempts'),
      );
      return;
    }

    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts);
    this.reconnectAttempts++;

    console.log(
      `Scheduling WebSocket reconnection attempt ${this.reconnectAttempts} in ${delay}ms`,
    );

    setTimeout(() => {
      if (!this.isConnected) {
        console.log(
          `Attempting to reconnect WebSocket (attempt ${this.reconnectAttempts})`,
        );
        this.initWebSocket();
      }
    }, delay);
  }

  /**
   * Get current connection status
   */
  get connected(): boolean {
    return this.isConnected;
  }

  /**
   * Close connection (alias for disconnect for compatibility)
   */
  close(): void {
    this.disconnect();
  }
}
