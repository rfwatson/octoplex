//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"regexp"
	"sync"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	pb "git.netflux.io/rob/octoplex/internal/generated/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// TODO: test all-in-one mode
func TestIntegrationRunServer(t *testing.T) {
	testCases := []struct {
		name              string
		args              []string
		wantStderr        []string
		wantStderrOnClose []string
		wantPorts         []int
	}{
		{
			name:      "launch server",
			args:      []string{"octoplex", "server", "start"},
			wantPorts: []int{50051, 1935},
			wantStderr: []string{
				`level=INFO msg="Starting server"`,
				`level=INFO msg="gRPC server started" component=server listen-addr=127.0.0.1:50051`,
				`level=INFO msg="Starting media server" component=server component=mediaserver host=localhost rtmp.enabled=true rtmp.bind_addr=127.0.0.1 rtmp.bind_port=1935 rtmps.enabled=true rtmps.bind_addr=127.0.0.1`,
				`level=INFO msg="Started container"`,
			},
			wantStderrOnClose: []string{
				`level=INFO msg="Stopping container"`,
				`level=INFO msg="Removing container"`,
			},
		},
		{
			name:      "launch server with custom gRPC listen address",
			args:      []string{"octoplex", "server", "start", "--listen-addr", ":30123"},
			wantPorts: []int{30123, 1935},
			wantStderr: []string{
				`level=INFO msg="Starting server"`,
				`level=INFO msg="gRPC server started" component=server listen-addr=[::]:30123`,
				`level=INFO msg="Starting media server" component=server component=mediaserver host=localhost rtmp.enabled=true rtmp.bind_addr=127.0.0.1 rtmp.bind_port=1935 rtmps.enabled=true rtmps.bind_addr=127.0.0.1`,
				`level=INFO msg="Started container"`,
			},
			wantStderrOnClose: []string{
				`level=INFO msg="Stopping container"`,
				`level=INFO msg="Removing container"`,
			},
		},
		{
			name:      "launch server with custom RTMP listen addresses",
			args:      []string{"octoplex", "server", "start", "--rtmp-listen-addr", "127.0.0.1:31935", "--rtmps-listen-addr", "0.0.0.0:31936"},
			wantPorts: []int{50051, 31935, 31936},
			wantStderr: []string{
				`level=INFO msg="Starting server"`,
				`level=INFO msg="gRPC server started" component=server listen-addr=127.0.0.1:50051`,
				`level=INFO msg="Starting media server" component=server component=mediaserver host=localhost rtmp.enabled=true rtmp.bind_addr=127.0.0.1 rtmp.bind_port=31935 rtmps.enabled=true rtmps.bind_addr=0.0.0.0 rtmps.bind_port=31936`,
				`level=INFO msg="Started container"`,
			},
			wantStderrOnClose: []string{
				`level=INFO msg="Stopping container"`,
				`level=INFO msg="Removing container"`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			done := make(chan struct{})

			var stdout, stderr concurrentBuffer

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			go func() {
				err = run(ctx, &stdout, &stderr, tc.args)

				done <- struct{}{}
			}()

			require.EventuallyWithT(
				t,
				func(c *assert.CollectT) {
					for _, port := range tc.wantPorts {
						// For now, just verify the ports are open.
						// TODO: connect with gRPC/RTMP clients
						var conn net.Conn
						conn, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
						require.NoError(c, err, "failed to connect to port %d", port)
						require.NoError(t, conn.Close(), "failed to close connection to port %d", port)
					}

					for _, want := range tc.wantStderr {
						assert.Contains(c, stderr.String(), want)
					}
				},
				10*time.Second,
				250*time.Millisecond,
			)

			cancel()
			<-done

			require.ErrorIs(t, err, context.Canceled)

			for _, want := range tc.wantStderrOnClose {
				assert.Contains(t, stderr.String(), want)
			}
		})
	}
}

func TestIntegrationClientServerUnary(t *testing.T) {
	type serverArgs struct {
		argv       func(*testing.T, string) []string
		wantStdout string
		wantErr    string
	}

	type clientArgs struct {
		argv       func(*testing.T, string) []string
		wantStdout string
		wantErr    string
	}

	testCases := []struct {
		name         string
		serverArgs   serverArgs
		clientArgs   clientArgs
		authenticate bool
	}{
		{
			name: "auth mode unspecified, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
			},
			authenticate: false,
		},
		{
			name: "auth mode unspecified, non-localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
				wantErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			},
			authenticate: false,
		},
		{
			name: "auth mode unspecified, non-localhost, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, apiToken string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify", "--api-token", apiToken}
				},
			},
			authenticate: true,
		},
		{
			name: "auth mode none, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "none"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
			},
			authenticate: false,
		},
		{
			name: "auth mode none, non-localhost, no insecure-allow-no-auth passed to server",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "none"}
				},
				wantErr: "new server: build credentials: authentication cannot be disabled", // handled in main.go
			},
			authenticate: false,
		},
		{
			name: "auth mode none, non-localhost, insecure-allow-no-auth passed to server",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "none", "--insecure-allow-no-auth"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
			},
			authenticate: false,
		},
		{
			name: "auth mode auto, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
			},
			authenticate: false,
		},
		{
			name: "auth mode auto, non-localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
				wantErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			},
			authenticate: false,
		},
		{
			name: "auth mode auto, non-localhost, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, apiToken string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify", "--api-token", apiToken}
				},
			},
			authenticate: true,
		},
		{
			name: "auth mode token, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "token"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, _ string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify"}
				},
				wantErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			},
			authenticate: false,
		},
		{
			name: "auth mode token, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "token"}
				},
				wantErr: "context canceled",
			},
			clientArgs: clientArgs{
				argv: func(t *testing.T, apiToken string) []string {
					return []string{"octoplex", "client", "destination", "list", "--host", "localhost:50051", "--tls-skip-verify", "--api-token", apiToken}
				},
			},
			authenticate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			dataDir := t.TempDir()
			var srvStdout, srvStderr concurrentBuffer

			done := make(chan struct{})
			go func() {
				defer close(done)

				err := run(ctx, &srvStdout, &srvStderr, tc.serverArgs.argv(t, dataDir))
				if tc.serverArgs.wantErr == "" {
					assert.NoError(t, err)
				} else {
					assert.ErrorContains(t, err, tc.serverArgs.wantErr)
				}
			}()

			if tc.clientArgs.argv != nil {
				require.EventuallyWithT(
					t,
					func(c *assert.CollectT) {
						assert.Contains(c, srvStderr.String(), `level=INFO msg="gRPC server started"`)
					},
					5*time.Second,
					250*time.Millisecond,
				)

				// Extract API tokenfrom the log, if it exists
				var apiToken string
				regex := regexp.MustCompile("TOKEN: (?P<token>[0-9a-f]{64})")
				if matches := regex.FindStringSubmatch(srvStderr.String()); len(matches) > 1 {
					apiToken = matches[1]
					t.Log("Detected API token:", apiToken, "will send:", tc.authenticate)
				}

				var apiTokenToSend string
				if tc.authenticate {
					apiTokenToSend = apiToken
				}

				var clientStdout, clientStderr concurrentBuffer
				err := run(ctx, &clientStdout, &clientStderr, tc.clientArgs.argv(t, apiTokenToSend))
				if tc.clientArgs.wantErr == "" {
					assert.NoError(t, err)

					if tc.clientArgs.wantStdout != "" {
						assert.Contains(t, clientStdout.String(), tc.clientArgs.wantStdout)
					}
				} else {
					assert.ErrorContains(t, err, tc.clientArgs.wantErr)
				}
			}

			cancel()

			<-done
		})
	}
}

func TestIntegrationClientServerStream(t *testing.T) {
	type serverArgs struct {
		argv       func(*testing.T, string) []string
		wantStdout string
		wantErr    string
	}

	type clientArgs struct {
		argv       func(*testing.T, string) []string
		wantStdout string
		wantErr    string
	}

	testCases := []struct {
		name          string
		serverArgs    serverArgs
		skipClient    bool
		wantClientErr string
		authenticate  bool
	}{
		{
			name: "auth mode unspecified, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			authenticate: false,
		},
		{
			name: "auth mode unspecified, non-localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			wantClientErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			authenticate:  false,
		},
		{
			name: "auth mode unspecified, non-localhost, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir}
				},
				wantErr: "context canceled",
			},
			authenticate: true,
		},
		{
			name: "auth mode none, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "none"}
				},
				wantErr: "context canceled",
			},
			authenticate: false,
		},
		{
			name: "auth mode none, non-localhost, no insecure-allow-no-auth passed to server",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "none"}
				},
				wantErr: "new server: build credentials: authentication cannot be disabled", // handled in main.go
			},
			skipClient: true,
		},
		{
			name: "auth mode none, non-localhost, insecure-allow-no-auth passed to server",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "none", "--insecure-allow-no-auth"}
				},
				wantErr: "context canceled",
			},
			authenticate: false,
		},
		{
			name: "auth mode auto, localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			authenticate: false,
		},
		{
			name: "auth mode auto, non-localhost, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			wantClientErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			authenticate:  false,
		},
		{
			name: "auth mode auto, non-localhost, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", ":50051", "--data-dir", dataDir, "--auth", "auto"}
				},
				wantErr: "context canceled",
			},
			authenticate: true,
		},
		{
			name: "auth mode token, no authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "token"}
				},
				wantErr: "context canceled",
			},
			wantClientErr: "rpc error: code = Unauthenticated desc = invalid credentials",
			authenticate:  false,
		},
		{
			name: "auth mode token, authentication provided",
			serverArgs: serverArgs{
				argv: func(t *testing.T, dataDir string) []string {
					return []string{"octoplex", "server", "start", "--listen-addr", "127.0.0.1:50051", "--data-dir", dataDir, "--auth", "token"}
				},
				wantErr: "context canceled",
			},
			authenticate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			dataDir := t.TempDir()
			var srvStdout, srvStderr concurrentBuffer

			done := make(chan struct{})
			go func() {
				defer close(done)

				err := run(ctx, &srvStdout, &srvStderr, tc.serverArgs.argv(t, dataDir))
				if tc.serverArgs.wantErr == "" {
					assert.NoError(t, err)
				} else {
					assert.ErrorContains(t, err, tc.serverArgs.wantErr)
				}
			}()

			if !tc.skipClient {
				require.EventuallyWithT(
					t,
					func(c *assert.CollectT) {
						assert.Contains(c, srvStderr.String(), `level=INFO msg="gRPC server started"`)
					},
					5*time.Second,
					250*time.Millisecond,
				)

				// Extract API token from the log, if it exists
				var apiToken string
				regex := regexp.MustCompile("TOKEN: (?P<token>[0-9a-f]{64})")
				if matches := regex.FindStringSubmatch(srvStderr.String()); len(matches) > 1 {
					apiToken = matches[1]
					t.Log("Detected API token:", apiToken, "will send:", tc.authenticate)
				}

				var apiTokenToSend string
				if tc.authenticate {
					apiTokenToSend = apiToken
				}

				tlsConfig := &tls.Config{
					MinVersion:         config.TLSMinVersion,
					InsecureSkipVerify: true,
					NextProtos: []string{"h2"},
				}

				conn, err := grpc.NewClient(
					"localhost:50051",
					grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
					grpc.WithStreamInterceptor(authInterceptorStream(apiTokenToSend)),
				)
				require.NoError(t, err)
				defer conn.Close()

				apiClient := pb.NewInternalAPIClient(conn)
				stream, err := apiClient.Communicate(ctx)
				require.NoError(t, err, "failed to create stream")

				require.NoError(t, stream.Send(&pb.Envelope{Payload: &pb.Envelope_Command{Command: &pb.Command{CommandType: &pb.Command_StartHandshake{}}}}))

				_, err = stream.Recv()
				if tc.wantClientErr == "" {
					require.NoError(t, err)
				} else {
					assert.ErrorContains(t, err, tc.wantClientErr)
				}
			}

			cancel()

			<-done
		})
	}
}

func authInterceptorStream(apiToken string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if apiToken == "" {
			return streamer(ctx, desc, cc, method, opts...)
		}

		md := metadata.New(nil)
		md.Set("authorization", "Bearer "+apiToken)
		return streamer(metadata.NewOutgoingContext(ctx, md), desc, cc, method, opts...)
	}
}

type concurrentBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (cb *concurrentBuffer) Write(p []byte) (n int, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.buf.Write(p)
}

func (cb *concurrentBuffer) String() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.buf.String()
}
