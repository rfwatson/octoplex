package testhelpers

import (
	"log/slog"
	"os"
)

// NewNopLogger returns a logger that discards all log output.
func NewNopLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// NewTestLogger returns a logger that writes to stderr.
func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}
