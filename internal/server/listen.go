package server

import (
	"fmt"
	"net"
)

// ListenerFunc is a function that returns a [net.Listener].
type ListenerFunc func() (net.Listener, error)

// WithListener creates a TCP listener on the given address.
func Listener(addr string) func() (net.Listener, error) {
	return func() (net.Listener, error) {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen: %w", err)
		}
		return listener, nil
	}
}

// WithListener returns the provided listener.
func WithListener(lis net.Listener) func() (net.Listener, error) {
	return func() (net.Listener, error) {
		return lis, nil
	}
}
