package store_test

import (
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/stretchr/testify/require"
)

func TestTokenStore(t *testing.T) {
	dataDir := t.TempDir()
	logger := testhelpers.NewTestLogger(t)
	tokenStore := store.NewTokenStore(dataDir, logger)
	key := "test-token"

	_, err := tokenStore.Get(key)
	require.ErrorIs(t, err, store.ErrTokenNotFound)

	rawToken := token.RawToken([]byte("this is a test token"))
	testToken, err := token.New(rawToken, time.Time{})
	require.NoError(t, err)

	require.NoError(t, tokenStore.Put(key, testToken))

	fetchedToken, err := tokenStore.Get(key)
	require.NoError(t, err)
	require.Equal(t, testToken.Hashed, fetchedToken.Hashed)

	matches, err := token.Matches(fetchedToken, rawToken)
	require.NoError(t, err)
	require.True(t, matches)

	require.NoError(t, tokenStore.Delete(key))

	_, err = tokenStore.Get(key)
	require.ErrorIs(t, err, store.ErrTokenNotFound)
}
