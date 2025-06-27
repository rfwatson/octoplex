package client

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1"
	connectpb "git.netflux.io/rob/octoplex/internal/generated/grpc/internalapi/v1/internalapiv1connect"
	"git.netflux.io/rob/octoplex/internal/httphelpers"
	"git.netflux.io/rob/octoplex/internal/optional"
	"git.netflux.io/rob/octoplex/internal/protocol"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// DefaultServerAddr is the default address for the client to connect to.
const DefaultServerAddr = "localhost:8443"

// App is the client application.
type App struct {
	bus                *event.Bus
	serverAddr         string
	insecureSkipVerify bool
	apiToken           string
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
	APIToken           string
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
		apiToken:           params.APIToken,
		clipboardAvailable: params.ClipboardAvailable,
		buildInfo:          params.BuildInfo,
		screen:             params.Screen,
		logger:             params.Logger,
	}
}

// ListDestinations retrieves the list of destinations from the server.
func (a *App) ListDestinations(ctx context.Context) (_ []domain.Destination, err error) {
	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.ListDestinations(ctx, &connect.Request[pb.ListDestinationsRequest]{
		Msg: &pb.ListDestinationsRequest{
			Command: &pb.ListDestinationsCommand{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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
	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return "", fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.AddDestination(ctx, &connect.Request[pb.AddDestinationRequest]{
		Msg: &pb.AddDestinationRequest{
			Command: &pb.AddDestinationCommand{
				Name: name,
				Url:  url,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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

	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.UpdateDestination(ctx, &connect.Request[pb.UpdateDestinationRequest]{
		Msg: &pb.UpdateDestinationRequest{
			Command: &pb.UpdateDestinationCommand{
				Id:   id[:],
				Name: name.Ref(),
				Url:  url.Ref(),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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

	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.RemoveDestination(ctx, &connect.Request[pb.RemoveDestinationRequest]{
		Msg: &pb.RemoveDestinationRequest{
			Command: &pb.RemoveDestinationCommand{Id: id[:], Force: force},
		},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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

	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.StartDestination(ctx, &connect.Request[pb.StartDestinationRequest]{
		Msg: &pb.StartDestinationRequest{
			Command: &pb.StartDestinationCommand{Id: id[:]},
		},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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

	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}

	resp, err := apiClient.StopDestination(ctx, &connect.Request[pb.StopDestinationRequest]{
		Msg: &pb.StopDestinationRequest{
			Command: &pb.StopDestinationCommand{Id: id[:]},
		},
	})
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}

	switch evt := resp.Msg.Result.(type) {
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
	apiClient, err := a.buildAPIClient(ctx)
	if err != nil {
		return fmt.Errorf("build client conn: %w", err)
	}

	stream := apiClient.Communicate(ctx)

	// Perform a handshake with the server, which has the function of flushing
	// any authentication issues before the UI is initialised.
	if err := a.doHandshake(stream); err != nil {
		return fmt.Errorf("do handshake: %w", err)
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

	g, ctx := errgroup.WithContext(ctx)

	// If the context is cancelled, we must close the gRPC stream.
	// It seems that passing the context to apiClient.Communicate() is not
	// enough.
	g.Go(func() error {
		<-ctx.Done()

		if err := stream.CloseRequest(); err != nil {
			a.logger.Error("Error closing gRPC stream after context cancelled", "err", err)
		}

		return ctx.Err()
	})

	g.Go(func() error { return ui.Run(ctx) })

	g.Go(func() error {
		for {
			envelope, recErr := stream.Receive()
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

func (a *App) buildAPIClient(ctx context.Context) (connectpb.APIServiceClient, error) {
	if a.insecureSkipVerify {
		a.logger.Warn(
			"TLS certificate verification is DISABLED — traffic is encrypted but unauthenticated",
			"component", "grpc",
			"risk", "vulnerable to active MITM",
			"action", "use a trusted certificate and enable verification before production",
		)
	}

	httpClient, err := httphelpers.NewH2Client(ctx, a.serverAddr, a.insecureSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("new H2 client: %w", err)
	}

	client := connectpb.NewAPIServiceClient(
		httpClient,
		"https://"+a.serverAddr,
		connect.WithInterceptors(authInterceptor{apiToken: a.apiToken}),
	)
	if err != nil {
		return nil, fmt.Errorf("new API client: %w", err)
	}

	return client, nil
}

func (a *App) doHandshake(stream *connect.BidiStreamForClient[pb.Envelope, pb.Envelope]) error {
	if err := stream.Send(&pb.Envelope{Payload: &pb.Envelope_Command{Command: &pb.Command{CommandType: &pb.Command_StartHandshake{}}}}); err != nil {
		return fmt.Errorf("send start handshake command: %w", err)
	}

	env, err := stream.Receive()
	if err != nil {
		return fmt.Errorf("receive handshake completed event: %w", err)
	}
	if evt := env.GetEvent(); evt == nil || evt.GetHandshakeCompleted() == nil {
		return fmt.Errorf("expected handshake completed event but got: %T", env)
	}

	return nil
}

type authInterceptor struct {
	apiToken string
}

func NewAuthInterceptor(apiToken string) *authInterceptor {
	return &authInterceptor{apiToken: apiToken}
}

func (a authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if a.apiToken != "" {
			req.Header().Set("authorization", "Bearer "+a.apiToken)
		}

		return next(ctx, req)
	}
}

func (a authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return connect.StreamingClientFunc(func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("authorization", "Bearer "+a.apiToken)

		return conn
	})
}

func (a authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		// no-op
		return next(ctx, conn)
	})
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
