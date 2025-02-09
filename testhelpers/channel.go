package testhelpers

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

// ChanDiscard consumes a channel and discards all values.
func ChanDiscard[T any](ch <-chan T) {
	go func() {
		for range ch {
			// no-op
		}
	}()
}

// ChanRequireNoError consumes a channel and asserts that no error is received.
func ChanRequireNoError(ctx context.Context, t testing.TB, ch <-chan error) {
	t.Helper()

	go func() {
		for {
			select {
			case err := <-ch:
				require.NoError(t, err)
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// ChanLog logs a channel's values.
func ChanLog[T any](ch <-chan T, logger *slog.Logger) {
	go func() {
		for v := range ch {
			logger.Info("Channel", "value", v)
		}
	}()
}
