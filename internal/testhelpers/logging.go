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
	var handlerOpts slog.HandlerOptions
	// RUNNER_DEBUG is used in the GitHub actions runner to enable debug logging.
	if os.Getenv("DEBUG") != "" || os.Getenv("RUNNER_DEBUG") != "" {
		handlerOpts.Level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &handlerOpts))
}
