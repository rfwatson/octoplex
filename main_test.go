//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: test client
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
