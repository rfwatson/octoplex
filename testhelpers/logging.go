package testhelpers

import (
	"io"
	"log/slog"
	"os"
)

// NewNopLogger returns a logger that discards all log output.
//
// TODO: remove in Go 1.24: https://github.com/golang/go/issues/62005
func NewNopLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// NewTestLogger returns a logger that writes to stderr.
func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
