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
		wantCredentials     bool
		wantErr             string
	}{
		{
			name:            "existing token, auth mode token",
			existingToken:   "s3cr3t",
			authMode:        config.AuthModeToken,
			wantCredentials: true,
		},
		{
			name:            "existing token, auth mode none",
			existingToken:   "s3cr3t",
			authMode:        config.AuthModeNone,
			wantCredentials: true,
		},
		{
			name:     "no existing token, auth mode none, no insecure allow no auth",
			authMode: config.AuthModeNone,
			wantErr:  "no token found and authentication is required, please run the server with --insecure-allow-no-auth to disable authentication",
		},
		{
			name:                "no existing token, auth mode none, insecure allow no auth",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantCredentials:     false,
		},
		{
			name:            "no existing token, auth mode auto, localhost address",
			listenAddr:      "127.0.0.1:50051",
			authMode:        config.AuthModeAuto,
			wantCredentials: false,
		},
		{
			name:            "no existing token, auth mode auto, non-localhost address",
			listenAddr:      "192.168.1.100:50051",
			authMode:        config.AuthModeAuto,
			wantCredentials: true,
		},
		{
			name:            "no existing token, auth mode token, localhost address",
			listenAddr:      "127.0.0.1:50051",
			authMode:        config.AuthModeToken,
			wantCredentials: true,
		},
		{
			name:            "no existing token, auth mode token, non-localhost address",
			listenAddr:      "192.168.1.100:50051",
			authMode:        config.AuthModeToken,
			wantCredentials: true,
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
				assert.Equal(t, tc.wantCredentials, !got.disabled)
			}
		})
	}
}
