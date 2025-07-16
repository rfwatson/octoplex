package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// WebSocketProxy bridges WebSocket connections to ConnectRPC bidirectional streams.
type WebSocketProxy struct {
	server          *Server
	credentialsMode CredentialsMode
	tokenStore      TokenStore
	upgrader        websocket.Upgrader
	logger          *slog.Logger

	mu          sync.RWMutex
	connections map[*websocket.Conn]context.CancelFunc
}

// newWebSocketProxy creates a new WebSocket proxy.
func newWebSocketProxy(cfg config.Config, server *Server, credentialsMode CredentialsMode, tokenStore TokenStore, logger *slog.Logger) *WebSocketProxy {
	return &WebSocketProxy{
		server:          server,
		credentialsMode: credentialsMode,
		tokenStore:      tokenStore,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == strings.TrimSuffix(cfg.ServerURL.BaseURL, "/")
			},
		},
		logger:      logger.With("component", "websocket_proxy"),
		connections: make(map[*websocket.Conn]context.CancelFunc),
	}
}

// Handler handles incoming WebSocket connections.
func (p *WebSocketProxy) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := p.upgrader.Upgrade(w, r, nil)
		if err != nil {
			p.logger.Error("Failed to upgrade WebSocket connection", "error", err)
			return
		}

		if p.credentialsMode != CredentialsModeDisabled {
			if ok := isAuthenticatedWithSessionToken(r.Header.Get("cookie"), p.tokenStore, p.logger); !ok {
				p.handleUnauthorized(r, conn)
				return
			}
		}

		p.logger.Info("WebSocket client connected", "remote_addr", r.RemoteAddr)

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		p.mu.Lock()
		p.connections[conn] = cancel
		p.mu.Unlock()

		p.handleConnection(ctx, conn)

		p.mu.Lock()
		delete(p.connections, conn)
		p.mu.Unlock()

		if err := conn.Close(); err != nil {
			p.logger.Error("Failed to close WebSocket connection", "error", err)
		}
		cancel()

		p.logger.Info("WebSocket client disconnected", "remote_addr", r.RemoteAddr)
	}
}

func (p *WebSocketProxy) handleUnauthorized(r *http.Request, conn *websocket.Conn) {
	p.logger.Warn("Unauthorized WebSocket connection attempt", "remote_addr", r.RemoteAddr)

	const websocketCloseCodeUnauthorized = 4003
	if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocketCloseCodeUnauthorized, "unauthorized"), time.Now().Add(time.Second)); err != nil {
		p.logger.Error("Failed to send unauthorized WebSocket close message", "error", err)
		return
	}

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		p.logger.Error("Failed to set read deadline for unauthorized WebSocket connection", "error", err)
		return
	}

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	if err := conn.Close(); err != nil {
		p.logger.Error("Failed to close unauthorized WebSocket connection", "error", err)
	}
}

// handleConnection manages the lifecycle of a single WebSocket connection.
func (p *WebSocketProxy) handleConnection(ctx context.Context, conn *websocket.Conn) {
	const chanSize = 10

	// commandC is for passing commands from websocket to the app.
	commandC := make(chan *pb.Envelope, chanSize)

	// eventC is for passing events from the app to websocket.
	eventC := make(chan *pb.Envelope, chanSize)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		p.readWebSocketMessages(ctx, conn, commandC)
	}()

	go func() {
		defer wg.Done()
		p.writeWebSocketMessages(ctx, conn, eventC)
	}()

	go func() {
		defer wg.Done()
		p.processMessages(ctx, commandC, eventC)
	}()

	wg.Wait()
}

// readWebSocketMessages reads messages from WebSocket and forwards them as commands.
func (p *WebSocketProxy) readWebSocketMessages(ctx context.Context, conn *websocket.Conn, commandC chan<- *pb.Envelope) {
	defer close(commandC)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, messageBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					p.logger.Error("WebSocket read error", "error", err)
				}
				return
			}

			var envelope pb.Envelope
			if err := proto.Unmarshal(messageBytes, &envelope); err != nil {
				p.logger.Error("Failed to parse WebSocket message", "error", err)
				return
			}

			select {
			case commandC <- &envelope:
			case <-ctx.Done():
				return
			}
		}
	}
}

// writeWebSocketMessages writes events to the WebSocket connection.
func (p *WebSocketProxy) writeWebSocketMessages(ctx context.Context, conn *websocket.Conn, eventChan <-chan *pb.Envelope) {
	for {
		select {
		case <-ctx.Done():
			return
		case envelope, ok := <-eventChan:
			if !ok {
				return
			}

			messageBytes, err := proto.Marshal(envelope)
			if err != nil {
				p.logger.Error("Failed to marshal WebSocket message", "error", err)
				return
			}

			if err = conn.WriteMessage(websocket.BinaryMessage, messageBytes); err != nil {
				p.logger.Error("WebSocket write error", "error", err)
				return
			}
		}
	}
}

// processMessages handles the command/event processing logic.
func (p *WebSocketProxy) processMessages(ctx context.Context, commandChan <-chan *pb.Envelope, eventChan chan<- *pb.Envelope) {
	defer close(eventChan)

	var handshakeCompleted bool
	var clientID event.ClientID
	var eventsC <-chan event.Event

	const handshakeTimeout = 5 * time.Second
	handshakeInterval := time.NewTimer(handshakeTimeout)

	for {
		select {
		case <-ctx.Done():
			return
		case envelope, ok := <-commandChan:
			if !ok {
				return
			}

			if envelope.GetCommand() != nil {
				command := envelope.GetCommand()

				if command.GetStartHandshake() != nil {
					if handshakeCompleted {
						p.logger.Error("Received StartHandshake command after handshake already completed")
						return
					}
					handshakeCompleted = true
					handshakeInterval.Stop()

					env := &pb.Envelope{
						Payload: &pb.Envelope_Event{
							Event: &pb.Event{
								EventType: &pb.Event_HandshakeCompleted{
									HandshakeCompleted: &pb.HandshakeCompletedEvent{},
								},
							},
						},
					}

					select {
					case eventChan <- env:
					case <-ctx.Done():
						return
					}

					// Handshake complete, register to receive events:
					clientID, eventsC = p.server.bus.Register()
					defer p.server.bus.Deregister(clientID)

					continue
				}

				// Dispatch other commands to the server.
				cmd, err := protocol.CommandFromWrappedProto(command)
				if err != nil {
					p.logger.Error("Failed to convert command", "error", err)
					continue
				}

				p.server.dispatchAsync(clientID, cmd)
			}
		case <-handshakeInterval.C:
			p.logger.Warn("Handshake timeout, closing connection")
			return

		case evt, ok := <-eventsC:
			if !ok {
				return
			}

			// Convert internal event to protobuf envelope using the same logic as the main server
			protoEvent := protocol.EventToWrappedProto(evt)
			envelope := &pb.Envelope{
				Payload: &pb.Envelope_Event{
					Event: protoEvent,
				},
			}

			select {
			case eventChan <- envelope:
			case <-ctx.Done():
				return
			}
		}
	}
}

// GetConnectionCount returns the number of active WebSocket connections.
func (p *WebSocketProxy) GetConnectionCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}
