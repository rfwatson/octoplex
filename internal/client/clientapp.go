package client

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/optional"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DefaultServerAddr is the default address for the client to connect to.
const DefaultServerAddr = "localhost:50051"

// App is the client application.
type App struct {
	bus                *event.Bus
	serverAddr         string
	insecureSkipVerify bool
	clipboardAvailable bool
	buildInfo          domain.BuildInfo
	screen             *terminal.Screen
	logger             *slog.Logger
}

// NewParams contains the parameters for the App.
type NewParams struct {
	ClipboardAvailable bool
	ServerAddr         string
	InsecureSkipVerify bool
	BuildInfo          domain.BuildInfo
	Screen             *terminal.Screen
	Logger             *slog.Logger
}

// New creates a new App instance.
func New(params NewParams) *App {
	return &App{
		bus:                event.NewBus(params.Logger),
		serverAddr:         cmp.Or(params.ServerAddr, DefaultServerAddr),
		insecureSkipVerify: params.InsecureSkipVerify,
		clipboardAvailable: params.ClipboardAvailable,
		buildInfo:          params.BuildInfo,
		screen:             params.Screen,
		logger:             params.Logger,
	}
}

// ListDestinations retrieves the list of destinations from the server.
func (a *App) ListDestinations(ctx context.Context) (_ []domain.Destination, err error) {
	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.ListDestinations(ctx, &pb.ListDestinationsRequest{
		Command: &pb.ListDestinationsCommand{},
	})
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.ListDestinationsResponse_Ok:
		return protocol.ProtoToDestinations(evt.Ok.Destinations)
	case *pb.ListDestinationsResponse_Error:
		return nil, fmt.Errorf("list destinations failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

// AddDestination adds a new destination to the server.
func (a *App) AddDestination(ctx context.Context, name, url string) (id string, err error) {
	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return "", fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.AddDestination(ctx, &pb.AddDestinationRequest{
		Command: &pb.AddDestinationCommand{
			Name: name,
			Url:  url,
		},
	})
	if err != nil {
		return "", fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.AddDestinationResponse_Ok:
		destID, err := uuid.FromBytes(evt.Ok.Id)
		if err != nil {
			return "", fmt.Errorf("parse destination ID: %w", err)
		}

		return destID.String(), nil
	case *pb.AddDestinationResponse_Error:
		return "", fmt.Errorf("add destination failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

func (a *App) UpdateDestination(ctx context.Context, destinationID string, name, url optional.V[string]) error {
	id, err := parseID(destinationID)
	if err != nil {
		return fmt.Errorf("parse ID: %w", err)
	}

	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.UpdateDestination(ctx, &pb.UpdateDestinationRequest{
		Command: &pb.UpdateDestinationCommand{
			Id:   id[:],
			Name: name.Ref(),
			Url:  url.Ref(),
		},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.UpdateDestinationResponse_Ok:
		return nil
	case *pb.UpdateDestinationResponse_Error:
		return fmt.Errorf("update destination failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

func (a *App) RemoveDestination(ctx context.Context, destinationID string, force bool) error {
	id, err := parseID(destinationID)
	if err != nil {
		return fmt.Errorf("parse ID: %w", err)
	}

	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.RemoveDestination(ctx, &pb.RemoveDestinationRequest{
		Command: &pb.RemoveDestinationCommand{Id: id[:], Force: force},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.RemoveDestinationResponse_Ok:
		return nil
	case *pb.RemoveDestinationResponse_Error:
		return fmt.Errorf("remove destination failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

func (a *App) StartDestination(ctx context.Context, destinationID string) error {
	id, err := parseID(destinationID)
	if err != nil {
		return fmt.Errorf("parse ID: %w", err)
	}

	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.StartDestination(ctx, &pb.StartDestinationRequest{
		Command: &pb.StartDestinationCommand{Id: id[:]},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.StartDestinationResponse_Ok:
		return nil
	case *pb.StartDestinationResponse_Error:
		return fmt.Errorf("start destination failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

func (a *App) StopDestination(ctx context.Context, destinationID string) error {
	id, err := parseID(destinationID)
	if err != nil {
		return fmt.Errorf("parse ID: %w", err)
	}

	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)

	resp, err := apiClient.StopDestination(ctx, &pb.StopDestinationRequest{
		Command: &pb.StopDestinationCommand{Id: id[:]},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Result.(type) {
	case *pb.StopDestinationResponse_Ok:
		return nil
	case *pb.StopDestinationResponse_Error:
		return fmt.Errorf("stop destination failed: %s", evt.Error.Error)
	default:
		panic("unreachable")
	}
}

// Run starts the application, and blocks until it is closed.
//
// It returns nil if the application was closed by the user, or an error if it
// closed for any other reason.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	conn, err := a.buildClientConn(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}
	defer conn.Close()

	apiClient := pb.NewInternalAPIClient(conn)
	stream, err := apiClient.Communicate(ctx)
	if err != nil {
		return fmt.Errorf("create gRPC stream: %w", err)
	}

	ui, err := terminal.NewUI(ctx, terminal.Params{
		EventBus: a.bus,
		Dispatcher: func(cmd event.Command) {
			a.logger.Debug("Command dispatched to gRPC stream", "cmd", cmd.Name())
			if sendErr := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Command{Command: protocol.CommandToWrappedProto(cmd)}}); sendErr != nil {
				a.logger.Error("Error dispatching command to gRPC stream", "err", sendErr)
			}
		},
		ClipboardAvailable: a.clipboardAvailable,
		BuildInfo:          a.buildInfo,
		Screen:             a.screen,
		Logger:             a.logger.With("component", "ui"),
	})
	if err != nil {
		return fmt.Errorf("start terminal user interface: %w", err)
	}
	defer ui.Close()

	g.Go(func() error { return ui.Run(ctx) })

	// After the UI is available, perform a handshake with the server.
	// Ordering is important here. We want to ensure that the UI is ready to
	// react to events received from the server. Performing the handshake ensures
	// the client has received at least one event.
	if err := a.doHandshake(stream); err != nil {
		return fmt.Errorf("do handshake: %w", err)
	}

	g.Go(func() error {
		for {
			envelope, recErr := stream.Recv()
			if recErr != nil {
				return fmt.Errorf("recv: %w", recErr)
			}

			pbEvt := envelope.GetEvent()
			if pbEvt == nil {
				a.logger.Error("Received envelope without event")
				continue
			}

			evt, err := protocol.EventFromWrappedProto(pbEvt)
			if err != nil {
				a.logger.Error("Failed to convert protobuf event to domain event", "err", err, "event", pbEvt)
				continue
			}

			a.logger.Debug("Received event from gRPC stream", "event", evt.EventName(), "payload", evt)
			a.bus.Send(evt)
		}
	})

	if err := g.Wait(); err == terminal.ErrUserClosed {
		return nil
	} else {
		return fmt.Errorf("errgroup.Wait: %w", err)
	}
}

func (a *App) buildClientConn(ctx context.Context) (*grpc.ClientConn, error) {
	if a.insecureSkipVerify {
		a.logger.Warn(
			"TLS certificate verification is DISABLED — traffic is encrypted but unauthenticated",
			"component", "grpc",
			"risk", "vulnerable to active MITM",
			"action", "use a trusted certificate and enable verification before production",
		)
	}

	tlsConfig := &tls.Config{
		MinVersion:         config.TLSMinVersion,
		InsecureSkipVerify: a.insecureSkipVerify,
	}

	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Test the TLS config.
		// This returns a more meaningful error than if we wait for gRPC layer to
		// fail.
		var tlsConn *tls.Conn
		tlsConn, err = tls.Dial("tcp", a.serverAddr, tlsConfig)
		if err == nil {
			tlsConn.Close()
		}
	}()

	select {
	case <-done:
		if err != nil {
			return nil, fmt.Errorf("test TLS connection: %w", err)
		}
	case <-ctx.Done(): // Important: avoid blocking forever if the context is cancelled during startup.
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, errors.New("timed out waiting for TLS connection to be established")
	}

	conn, err := grpc.NewClient(
		a.serverAddr,
		grpc.WithTransportCredentials(
			credentials.NewTLS(tlsConfig),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("new gRPC client: %w", err)
	}

	return conn, nil
}

func (a *App) doHandshake(stream pb.InternalAPI_CommunicateClient) error {
	if err := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Command{Command: &pb.Command{CommandType: &pb.Command_StartHandshake{}}}}); err != nil {
		return fmt.Errorf("send start handshake command: %w", err)
	}

	env, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("receive handshake completed event: %w", err)
	}
	if evt := env.GetEvent(); evt == nil || evt.GetHandshakeCompleted() == nil {
		return fmt.Errorf("expected handshake completed event but got: %T", env)
	}

	return nil
}

func parseID(id string) (uuid.UUID, error) {
	if id == "" {
		return uuid.Nil, errors.New("ID cannot be empty")
	}

	destID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse ID: %w", err)
	}

	return destID, nil
}
