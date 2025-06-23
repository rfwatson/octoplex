package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
	connectpb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1/internalapiv1connect"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/token"
	"golang.org/x/sync/errgroup"
)

// Server is the gRPC server that handles incoming commands and outgoing
// events.
type Server struct {
	connectpb.UnimplementedAPIServiceHandler

	dispatchSync  func(event.Command) (event.Event, error)
	dispatchAsync func(event.ClientID, event.Command)
	bus           *event.Bus
	logger        *slog.Logger

	mu          sync.Mutex
	clientCount int
	clientC     chan struct{}
}

// newServer creates a new gRPC server.
func newServer(
	dispatchSync func(event.Command) (event.Event, error),
	dispatchAsync func(event.ClientID, event.Command),
	bus *event.Bus,
	logger *slog.Logger,
) *Server {
	return &Server{
		dispatchSync:  dispatchSync,
		dispatchAsync: dispatchAsync,
		bus:           bus,
		clientC:       make(chan struct{}, 1),
		logger:        logger.With("component", "server"),
	}
}

func (s *Server) Communicate(ctx context.Context, stream *connect.BidiStream[pb.Envelope, pb.Envelope]) error {
	g, ctx := errgroup.WithContext(ctx)

	s.logger.Info("Client connected", "remote_addr", stream.Peer().Addr)

	// perform handshake:
	startHandshakeCmd, err := stream.Receive()
	if err != nil {
		return fmt.Errorf("receive start handshake command: %w", err)
	}
	if startHandshakeCmd.GetCommand() == nil || startHandshakeCmd.GetCommand().GetStartHandshake() == nil {
		return fmt.Errorf("expected start handshake command but got: %T", startHandshakeCmd)
	}
	if err := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Event{Event: &pb.Event{EventType: &pb.Event_HandshakeCompleted{}}}}); err != nil {
		return fmt.Errorf("send handshake completed event: %w", err)
	}

	// Notify that a client has connected and completed the handshake.
	select {
	case s.clientC <- struct{}{}:
	default:
	}

	clientID, eventsC := s.bus.Register()
	g.Go(func() error {
		defer s.bus.Deregister(clientID)

		for {
			select {
			case evt := <-eventsC:
				if err := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Event{Event: protocol.EventToWrappedProto(evt)}}); err != nil {
					if ctxErr := ctx.Err(); ctxErr != nil {
						err = ctxErr
					}
					return fmt.Errorf("send event: %w", err)
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	g.Go(func() error {
		s.mu.Lock()
		s.clientCount++
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			s.clientCount--
			s.mu.Unlock()
		}()

		for {
			in, err := stream.Receive()
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					err = ctxErr
				}
				return fmt.Errorf("receive message: %w", err)
			}

			switch pbCmd := in.Payload.(type) {
			case *pb.Envelope_Command:
				cmd, err := protocol.CommandFromWrappedProto(pbCmd.Command)
				if err != nil {
					return fmt.Errorf("command from proto: %w", err)
				}
				s.logger.Debug("Received command from gRPC stream", "command", cmd.Name())
				s.dispatchAsync(clientID, cmd)
			default:
				return fmt.Errorf("expected command but got: %T", pbCmd)
			}
		}
	})

	if err := g.Wait(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.logger.Error("Client stream closed with error", "err", err)
		return fmt.Errorf("errgroup.Wait: %w", err)
	}

	s.logger.Info("Client stream closed")

	return nil
}

// GetClientCount returns the number of connected clients.
func (s *Server) GetClientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.clientCount
}

const waitForClientTimeout = 10 * time.Second

// WaitForClient waits for _any_ client to connect and complete the handshake.
// It times out if no client has connected after 10 seconds.
func (s *Server) WaitForClient(ctx context.Context) error {
	select {
	case <-s.clientC:
		return nil
	case <-time.After(waitForClientTimeout):
		return errors.New("timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) ListDestinations(ctx context.Context, req *connect.Request[pb.ListDestinationsRequest]) (*connect.Response[pb.ListDestinationsResponse], error) {
	cmd, err := protocol.CommandFromListDestinationsProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationsListedEvent:
		return &connect.Response[pb.ListDestinationsResponse]{
			Msg: &pb.ListDestinationsResponse{
				Result: &pb.ListDestinationsResponse_Ok{
					Ok: protocol.DestinationsListedEventToProto(e),
				},
			},
		}, nil
	case event.ListDestinationsFailedEvent:
		return &connect.Response[pb.ListDestinationsResponse]{
			Msg: &pb.ListDestinationsResponse{
				Result: &pb.ListDestinationsResponse_Error{
					Error: protocol.ListDestinationsFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) AddDestination(ctx context.Context, req *connect.Request[pb.AddDestinationRequest]) (*connect.Response[pb.AddDestinationResponse], error) {
	cmd, err := protocol.CommandFromAddDestinationProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationAddedEvent:
		return &connect.Response[pb.AddDestinationResponse]{
			Msg: &pb.AddDestinationResponse{
				Result: &pb.AddDestinationResponse_Ok{
					Ok: protocol.DestinationAddedEventToProto(e),
				},
			},
		}, nil
	case event.AddDestinationFailedEvent:
		return &connect.Response[pb.AddDestinationResponse]{
			Msg: &pb.AddDestinationResponse{
				Result: &pb.AddDestinationResponse_Error{
					Error: protocol.AddDestinationFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) UpdateDestination(ctx context.Context, req *connect.Request[pb.UpdateDestinationRequest]) (*connect.Response[pb.UpdateDestinationResponse], error) {
	cmd, err := protocol.CommandFromUpdateDestinationProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationUpdatedEvent:
		return &connect.Response[pb.UpdateDestinationResponse]{
			Msg: &pb.UpdateDestinationResponse{
				Result: &pb.UpdateDestinationResponse_Ok{
					Ok: protocol.DestinationUpdatedEventToProto(e),
				},
			},
		}, nil
	case event.UpdateDestinationFailedEvent:
		return &connect.Response[pb.UpdateDestinationResponse]{
			Msg: &pb.UpdateDestinationResponse{
				Result: &pb.UpdateDestinationResponse_Error{
					Error: protocol.UpdateDestinationFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) RemoveDestination(ctx context.Context, req *connect.Request[pb.RemoveDestinationRequest]) (*connect.Response[pb.RemoveDestinationResponse], error) {
	cmd, err := protocol.CommandFromRemoveDestinationProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationRemovedEvent:
		return &connect.Response[pb.RemoveDestinationResponse]{
			Msg: &pb.RemoveDestinationResponse{
				Result: &pb.RemoveDestinationResponse_Ok{
					Ok: protocol.DestinationRemovedEventToProto(e),
				},
			},
		}, nil
	case event.RemoveDestinationFailedEvent:
		return &connect.Response[pb.RemoveDestinationResponse]{
			Msg: &pb.RemoveDestinationResponse{
				Result: &pb.RemoveDestinationResponse_Error{
					Error: protocol.RemoveDestinationFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) StartDestination(ctx context.Context, req *connect.Request[pb.StartDestinationRequest]) (*connect.Response[pb.StartDestinationResponse], error) {
	cmd, err := protocol.CommandFromStartDestinationProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationStartedEvent:
		return &connect.Response[pb.StartDestinationResponse]{
			Msg: &pb.StartDestinationResponse{
				Result: &pb.StartDestinationResponse_Ok{
					Ok: protocol.DestinationStartedEventToProto(e),
				},
			},
		}, nil
	case event.StartDestinationFailedEvent:
		return &connect.Response[pb.StartDestinationResponse]{
			Msg: &pb.StartDestinationResponse{
				Result: &pb.StartDestinationResponse_Error{
					Error: protocol.StartDestinationFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) StopDestination(ctx context.Context, req *connect.Request[pb.StopDestinationRequest]) (*connect.Response[pb.StopDestinationResponse], error) {
	cmd, err := protocol.CommandFromStopDestinationProto(req.Msg.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationStoppedEvent:
		return &connect.Response[pb.StopDestinationResponse]{
			Msg: &pb.StopDestinationResponse{
				Result: &pb.StopDestinationResponse_Ok{
					Ok: protocol.DestinationStoppedEventToProto(e),
				},
			},
		}, nil
	case event.StopDestinationFailedEvent:
		return &connect.Response[pb.StopDestinationResponse]{
			Msg: &pb.StopDestinationResponse{
				Result: &pb.StopDestinationResponse_Error{
					Error: protocol.StopDestinationFailedEventToProto(e),
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

type authInterceptor struct {
	credentials apiCredentials
	logger      *slog.Logger
}

func newAuthInterceptor(credentials apiCredentials, logger *slog.Logger) authInterceptor {
	if credentials.hashedToken == "" && !credentials.disabled {
		panic("API authentication is enabled but no token is configured")
	}

	return authInterceptor{credentials: credentials, logger: logger}
}

func (a authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if ok, err := isAuthenticated(req.Header().Get("authorization"), a.credentials, a.logger); err != nil || !ok {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
		}

		return next(ctx, req)
	}
}

func (a authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		// no-op
		return next(ctx, spec)
	})
}

func (a authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if ok, err := isAuthenticated(conn.RequestHeader().Get("authorization"), a.credentials, a.logger); err != nil || !ok {
			return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
		}

		return next(ctx, conn)
	})
}

// isAuthenticated checks if the request is authenticated using the provided
// credentials. It returns true, nil if the request is authenticated. If the
// request is not authenticated it should return a gRPC status with an
// appropriate message. It is responsible for logging any significant errors
// that occur.
func isAuthenticated(authHeader string, credentials apiCredentials, logger *slog.Logger) (bool, error) {
	if credentials.disabled {
		return true, nil
	}

	if authHeader == "" {
		return false, connect.NewError(connect.CodeUnauthenticated, errors.New("no credentials provided"))
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid authorization header format: %s", authHeader))
	}
	rawToken := strings.TrimPrefix(authHeader, "Bearer ")

	if isValid, err := token.Compare(token.RawToken(rawToken), credentials.hashedToken); err != nil || !isValid {
		if err != nil {
			logger.Error("Error authenticating", "err", err, "raw_token", rawToken)
		}

		return false, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
	}

	return true, nil
}
