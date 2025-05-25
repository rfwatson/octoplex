package client

import (
	"cmp"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/terminal"
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

// Run starts the application, and blocks until it is closed.
//
// It returns nil if the application was closed by the user, or an error if it
// closed for any other reason.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	if a.insecureSkipVerify {
		a.logger.Warn(
			"TLS certificate verification is DISABLED — traffic is encrypted but unauthenticated",
			"component", "grpc",
			"risk", "vulnerable to active MITM",
			"action", "use a trusted certificate and enable verification before production",
		)
	}

	// Test the TLS config.
	// This returns a more meaningful error than if we wait for gRPC layer to
	// fail.
	tlsConfig := &tls.Config{
		MinVersion:         config.TLSMinVersion,
		InsecureSkipVerify: a.insecureSkipVerify,
	}
	tlsConn, err := tls.Dial("tcp", a.serverAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	tlsConn.Close()

	conn, err := grpc.NewClient(
		a.serverAddr,
		grpc.WithTransportCredentials(
			credentials.NewTLS(tlsConfig),
		),
	)
	if err != nil {
		return fmt.Errorf("new gRPC client: %w", err)
	}

	apiClient := pb.NewInternalAPIClient(conn)
	stream, err := apiClient.Communicate(ctx)
	if err != nil {
		return fmt.Errorf("create gRPC stream: %w", err)
	}

	ui, err := terminal.NewUI(ctx, terminal.Params{
		EventBus: a.bus,
		Dispatcher: func(cmd event.Command) {
			a.logger.Debug("Command dispatched to gRPC stream", "cmd", cmd.Name())
			if sendErr := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Command{Command: protocol.CommandToProto(cmd)}}); sendErr != nil {
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

			evt, err := protocol.EventFromProto(pbEvt)
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
