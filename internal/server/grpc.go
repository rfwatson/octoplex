package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/peer"
)

// Server is the gRPC server that handles incoming commands and outgoing
// events.
type Server struct {
	pb.UnimplementedInternalAPIServer

	dispatcher func(event.Command)
	bus        *event.Bus
	logger     *slog.Logger

	mu          sync.Mutex
	clientCount int
	clientC     chan struct{}
}

// newServer creates a new gRPC server.
func newServer(
	dispatcher func(event.Command),
	bus *event.Bus,
	logger *slog.Logger,
) *Server {
	return &Server{
		dispatcher: dispatcher,
		bus:        bus,
		clientC:    make(chan struct{}, 1),
		logger:     logger.With("component", "server"),
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

	g.Go(func() error {
		eventsC := s.bus.Register()
		defer s.bus.Deregister(eventsC)

		for {
			select {
			case evt := <-eventsC:
				if err := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Event{Event: protocol.EventToProto(evt)}}); err != nil {
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
			if err == io.EOF {
				s.logger.Info("Client disconnected")
				return err
			}

			if err != nil {
				return fmt.Errorf("receive message: %w", err)
			}

			switch pbCmd := in.Payload.(type) {
			case *pb.Envelope_Command:
				cmd := protocol.CommandFromProto(pbCmd.Command)
				s.logger.Debug("Received command from gRPC stream", "command", cmd.Name())
				s.dispatcher(cmd)
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
