package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/token"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Server is the gRPC server that handles incoming commands and outgoing
// events.
type Server struct {
	pb.UnimplementedInternalAPIServer

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

func (s *Server) Communicate(stream pb.InternalAPI_CommunicateServer) error {
	g, ctx := errgroup.WithContext(stream.Context())

	var remoteAddr string
	peer, ok := peer.FromContext(ctx)
	if ok {
		remoteAddr = peer.Addr.String()
	}

	s.logger.Info("Client connected", "remote_addr", cmp.Or(remoteAddr, "unknown"))

	// perform handshake:
	startHandshakeCmd, err := stream.Recv()
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
					if ctxErr := stream.Context().Err(); ctxErr != nil {
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
			in, err := stream.Recv()
			if err != nil {
				if ctxErr := stream.Context().Err(); ctxErr != nil {
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

func (s *Server) ListDestinations(ctx context.Context, req *pb.ListDestinationsRequest) (*pb.ListDestinationsResponse, error) {
	cmd, err := protocol.CommandFromListDestinationsProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationsListedEvent:
		return &pb.ListDestinationsResponse{
			Result: &pb.ListDestinationsResponse_Ok{
				Ok: protocol.DestinationsListedEventToProto(e),
			},
		}, nil
	case event.ListDestinationsFailedEvent:
		return &pb.ListDestinationsResponse{
			Result: &pb.ListDestinationsResponse_Error{
				Error: protocol.ListDestinationsFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) AddDestination(ctx context.Context, req *pb.AddDestinationRequest) (*pb.AddDestinationResponse, error) {
	cmd, err := protocol.CommandFromAddDestinationProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationAddedEvent:
		return &pb.AddDestinationResponse{
			Result: &pb.AddDestinationResponse_Ok{
				Ok: protocol.DestinationAddedEventToProto(e),
			},
		}, nil
	case event.AddDestinationFailedEvent:
		return &pb.AddDestinationResponse{
			Result: &pb.AddDestinationResponse_Error{
				Error: protocol.AddDestinationFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) UpdateDestination(ctx context.Context, req *pb.UpdateDestinationRequest) (*pb.UpdateDestinationResponse, error) {
	cmd, err := protocol.CommandFromUpdateDestinationProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationUpdatedEvent:
		return &pb.UpdateDestinationResponse{
			Result: &pb.UpdateDestinationResponse_Ok{
				Ok: protocol.DestinationUpdatedEventToProto(e),
			},
		}, nil
	case event.UpdateDestinationFailedEvent:
		return &pb.UpdateDestinationResponse{
			Result: &pb.UpdateDestinationResponse_Error{
				Error: protocol.UpdateDestinationFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) RemoveDestination(ctx context.Context, req *pb.RemoveDestinationRequest) (*pb.RemoveDestinationResponse, error) {
	cmd, err := protocol.CommandFromRemoveDestinationProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationRemovedEvent:
		return &pb.RemoveDestinationResponse{
			Result: &pb.RemoveDestinationResponse_Ok{
				Ok: protocol.DestinationRemovedEventToProto(e),
			},
		}, nil
	case event.RemoveDestinationFailedEvent:
		return &pb.RemoveDestinationResponse{
			Result: &pb.RemoveDestinationResponse_Error{
				Error: protocol.RemoveDestinationFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) StartDestination(ctx context.Context, req *pb.StartDestinationRequest) (*pb.StartDestinationResponse, error) {
	cmd, err := protocol.CommandFromStartDestinationProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationStartedEvent:
		return &pb.StartDestinationResponse{
			Result: &pb.StartDestinationResponse_Ok{
				Ok: protocol.DestinationStartedEventToProto(e),
			},
		}, nil
	case event.StartDestinationFailedEvent:
		return &pb.StartDestinationResponse{
			Result: &pb.StartDestinationResponse_Error{
				Error: protocol.StartDestinationFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func (s *Server) StopDestination(ctx context.Context, req *pb.StopDestinationRequest) (*pb.StopDestinationResponse, error) {
	cmd, err := protocol.CommandFromStopDestinationProto(req.Command)
	if err != nil {
		return nil, fmt.Errorf("command from proto: %w", err)
	}

	evt, err := s.dispatchSync(cmd)
	if err != nil {
		return nil, fmt.Errorf("dispatch command: %w", err)
	}

	switch e := evt.(type) {
	case event.DestinationStoppedEvent:
		return &pb.StopDestinationResponse{
			Result: &pb.StopDestinationResponse_Ok{
				Ok: protocol.DestinationStoppedEventToProto(e),
			},
		}, nil
	case event.StopDestinationFailedEvent:
		return &pb.StopDestinationResponse{
			Result: &pb.StopDestinationResponse_Error{
				Error: protocol.StopDestinationFailedEventToProto(e),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected event type: %T", e)
	}
}

func authInterceptorUnary(credentials apiCredentials, logger *slog.Logger) grpc.UnaryServerInterceptor {
	if credentials.hashedToken == "" && !credentials.disabled {
		panic("API authentication is enabled but no token is configured")
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if ok, err := isAuthenticated(ctx, credentials, logger); err != nil || !ok {
			return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
		}

		return handler(ctx, req)
	}
}

func authInterceptorStream(credentials apiCredentials, logger *slog.Logger) grpc.StreamServerInterceptor {
	if credentials.hashedToken == "" && !credentials.disabled {
		panic("API authentication is enabled but no token is configured")
	}

	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if ok, err := isAuthenticated(ss.Context(), credentials, logger); err != nil || !ok {
			// Force the server to flush the response, and give the client enough
			// time to receive it.
			// This is needed to avoid a race condition between the server and
			// clients both closing their connection simultaneously during an
			// authentication failure. It seems that if the client closes its
			// connection before the server has chance to respond then the client
			// receives an io.EOF instead of an unauthenticated response. This does
			// seem pretty hacky, there's probably a better way to do it.
			ss.SendHeader(metadata.MD{}) //nolint:errcheck
			time.Sleep(time.Millisecond * 100)

			return status.Errorf(codes.Unauthenticated, "invalid credentials")
		}

		return handler(srv, ss)
	}
}

// isAuthenticated checks if the request is authenticated using the provided
// credentials. It returns true, nil if the request is authenticated. If the
// request is not authenticated it should return a gRPC status with an
// appropriate message. It is responsible for logging any significant errors
// that occur.
func isAuthenticated(ctx context.Context, credentials apiCredentials, logger *slog.Logger) (bool, error) {
	if credentials.disabled {
		return true, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false, status.Errorf(codes.Unauthenticated, "missing metadata")
	}

	authHeader := md.Get("authorization")
	if len(authHeader) < 1 {
		return false, status.Errorf(codes.Unauthenticated, "authorization token not supplied")
	}

	authHeaderValue := authHeader[0]
	if !strings.HasPrefix(authHeaderValue, "Bearer ") {
		return false, status.Errorf(codes.Unauthenticated, "invalid authorization format")
	}
	rawToken := strings.TrimPrefix(authHeaderValue, "Bearer ")

	if isValid, err := token.Compare(token.RawToken(rawToken), credentials.hashedToken); err != nil || !isValid {
		if err != nil {
			logger.Error("Error authenticating", "err", err, "raw_token", rawToken)
		}
		return false, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}

	return true, nil
}
