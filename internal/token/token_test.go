package token_test

import (
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToken(t *testing.T) {
	rawToken, _ := token.GenerateRawToken(32)
	userPassword := token.RawToken("letmein123")

	testCases := []struct {
		name           string
		rawToken       token.RawToken
		presentedToken token.RawToken
		expiresAt      time.Time
		wantMatch      bool
		wantErr        error
	}{
		{
			name:           "generated automatically, no expiry, valid token passed",
			rawToken:       rawToken,
			presentedToken: rawToken,
			wantMatch:      true,
		},
		{
			name:           "generated automatically, expiry in the future, valid token passed",
			rawToken:       rawToken,
			expiresAt:      time.Now().Add(24 * time.Hour),
			presentedToken: rawToken,
			wantMatch:      true,
		},
		{
			name:           "generated automatically, expiry in the past, valid token passed",
			rawToken:       rawToken,
			expiresAt:      time.Now().Add(-time.Minute),
			presentedToken: rawToken,
			wantMatch:      false,
			wantErr:        token.ErrTokenExpired,
		},
		{
			name:           "generated automatically, no expiry, invalid token passed",
			rawToken:       rawToken,
			presentedToken: []byte("nope"),
			wantMatch:      false,
		},
		{
			name:           "generated automatically, expiry in the future, invalid token passed",
			rawToken:       rawToken,
			expiresAt:      time.Now().Add(time.Minute),
			presentedToken: []byte("nope"),
			wantMatch:      false,
		},
		{
			name:           "generated automatically, expiry in the past, invalid token passed",
			rawToken:       rawToken,
			expiresAt:      time.Now().Add(-time.Minute),
			presentedToken: []byte("nope"),
			wantMatch:      false,
			wantErr:        token.ErrTokenExpired,
		},
		{
			name:           "user password, no expiry, valid token passed",
			rawToken:       userPassword,
			presentedToken: userPassword,
			wantMatch:      true,
		},
		{
			name:           "user password, expiry in the future, valid token passed",
			rawToken:       userPassword,
			expiresAt:      time.Now().Add(24 * time.Hour),
			presentedToken: userPassword,
			wantMatch:      true,
		},
		{
			name:           "user password, expiry in the past, valid token passed",
			rawToken:       userPassword,
			expiresAt:      time.Now().Add(-time.Minute),
			presentedToken: userPassword,
			wantMatch:      false,
			wantErr:        token.ErrTokenExpired,
		},
		{
			name:           "user password, no expiry, invalid token passed",
			rawToken:       userPassword,
			presentedToken: []byte("nope"),
			wantMatch:      false,
		},
		{
			name:           "user password, expiry in the future, invalid token passed",
			rawToken:       userPassword,
			expiresAt:      time.Now().Add(time.Minute),
			presentedToken: []byte("nope"),
			wantMatch:      false,
		},
		{
			name:           "user password, expiry in the past, invalid token passed",
			rawToken:       userPassword,
			expiresAt:      time.Now().Add(-time.Minute),
			presentedToken: []byte("nope"),
			wantMatch:      false,
			wantErr:        token.ErrTokenExpired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			generatedToken, err := token.New(tc.rawToken, tc.expiresAt)
			require.NoError(t, err)

			got, err := token.Matches(generatedToken, tc.presentedToken)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.False(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantMatch, got)
			}
		})
	}
}
