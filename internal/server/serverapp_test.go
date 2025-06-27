package server

import (
	"os"
	"path/filepath"
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCredentials(t *testing.T) {
	testCases := []struct {
		name                string
		existingToken       string
		listenAddr          string
		authMode            config.AuthMode
		insecureAllowNoAuth bool
		wantAuthRequired    bool
		wantErr             string
	}{
		{
			name:             "existing token, auth mode token",
			listenAddr:       ":8443",
			existingToken:    "s3cr3t",
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
		{
			name:             "existing token, auth mode none",
			listenAddr:       ":8443",
			existingToken:    "s3cr3t",
			authMode:         config.AuthModeNone,
			wantAuthRequired: true,
		},
		{
			name:                "existing token, auth mode none, localhost, insecure allow no auth",
			listenAddr:          "127.0.0.1:8443",
			existingToken:       "s3cr3t",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    false,
		},
		{
			name:                "existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddr:          ":8443",
			existingToken:       "s3cr3t",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    true,
		},
		{
			name:             "no existing token, auth mode none, localhost, no insecure allow no auth",
			listenAddr:       "127.0.0.1:8443",
			authMode:         config.AuthModeNone,
			wantAuthRequired: false,
		},
		{
			name:       "no existing token, auth mode none, non-localhost, no insecure allow no auth",
			listenAddr: "0.0.0.0:8443",
			authMode:   config.AuthModeNone,
			wantErr:    ErrAuthenticationCannotBeDisabled.Error(),
		},
		{
			name:                "no existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddr:          ":8443",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    false,
		},
		{
			name:             "no existing token, auth mode auto, localhost address",
			listenAddr:       "127.0.0.1:8443",
			authMode:         config.AuthModeAuto,
			wantAuthRequired: false,
		},
		{
			name:             "no existing token, auth mode auto, non-localhost address",
			listenAddr:       "192.168.1.100:8443",
			authMode:         config.AuthModeAuto,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode token, localhost address",
			listenAddr:       "127.0.0.1:8443",
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode token, non-localhost address",
			listenAddr:       "192.168.1.100:8443",
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			logger := testhelpers.NewTestLogger(t)
			cfg := config.Config{
				DataDir:             tempDir,
				ListenAddr:          tc.listenAddr,
				AuthMode:            tc.authMode,
				InsecureAllowNoAuth: tc.insecureAllowNoAuth,
			}

			if tc.existingToken != "" {
				require.NoError(t, os.WriteFile(filepath.Join(tempDir, "token.txt"), []byte(tc.existingToken), 0600))
			}

			got, err := buildCredentials(cfg, logger)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantAuthRequired, !got.disabled)
			}
		})
	}
}
