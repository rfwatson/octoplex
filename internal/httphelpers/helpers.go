package httphelpers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"golang.org/x/net/http2"
)

// NewH2Client creates a new HTTP/2 client with the specified address and TLS
// configuration. It is intended to be used for making gRPC requests over
// HTTP/2.
func NewH2Client(ctx context.Context, addr string, tlsSkipVerify bool) (*http.Client, error) {
	tlsConfig := &tls.Config{
		MinVersion:         config.TLSMinVersion,
		InsecureSkipVerify: tlsSkipVerify,
		NextProtos:         []string{"h2"},
	}

	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Test the TLS config.
		// This returns a more meaningful error than if we wait for gRPC layer to
		// fail.
		var tlsConn *tls.Conn
		tlsConn, err = tls.Dial("tcp", addr, tlsConfig)
		if err == nil {
			tlsConn.Close() //nolint:errcheck
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

	transport := &http.Transport{TLSClientConfig: tlsConfig}
	if err = http2.ConfigureTransport(transport); err != nil {
		return nil, fmt.Errorf("configure HTTP/2 transport: %w", err)
	}

	return &http.Client{
		// TODO: set timeout.
		// This needs to be configurable, for streams it must be long enough to
		// allow for the stream to complete.
		Transport: transport,
	}, nil
}
