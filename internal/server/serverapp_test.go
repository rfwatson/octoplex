package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCredentials(t *testing.T) {
	testCases := []struct {
		name                string
		existingToken       string
		listenAddrs         config.ListenAddrs
		authMode            config.AuthMode
		insecureAllowNoAuth bool
		wantAuthRequired    bool
		wantErr             string
	}{
		{
			name:             "existing token, auth mode token",
			listenAddrs:      config.ListenAddrs{TLS: ":8443"},
			existingToken:    "abcdef123",
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
		{
			name:             "existing token, auth mode none",
			listenAddrs:      config.ListenAddrs{TLS: ":8443"},
			existingToken:    "abcdef123",
			authMode:         config.AuthModeNone,
			wantAuthRequired: true,
		},
		{
			name:                "existing token, auth mode none, localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: "127.0.0.1:8443"},
			existingToken:       "abcdef123",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    false,
		},
		{
			name:                "existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: ":8443"},
			existingToken:       "abcdef123",
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    true,
		},
		{
			name:             "no existing token, auth mode none, localhost, no insecure allow no auth",
			listenAddrs:      config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:         config.AuthModeNone,
			wantAuthRequired: false,
		},
		{
			name:        "no existing token, auth mode none, non-localhost, no insecure allow no auth",
			listenAddrs: config.ListenAddrs{TLS: "0.0.0.0:8443"},
			authMode:    config.AuthModeNone,
			wantErr:     ErrAuthenticationCannotBeDisabled.Error(),
		},
		{
			name:                "no existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: ":8443"},
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			wantAuthRequired:    false,
		},
		{
			name:             "no existing token, auth mode auto, localhost address",
			listenAddrs:      config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:         config.AuthModeAuto,
			wantAuthRequired: false,
		},
		{
			name:             "no existing token, auth mode auto, non-localhost address",
			listenAddrs:      config.ListenAddrs{TLS: "192.168.1.100:8443"},
			authMode:         config.AuthModeAuto,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode token, localhost address",
			listenAddrs:      config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode token, non-localhost address",
			listenAddrs:      config.ListenAddrs{TLS: "192.168.1.100:8443"},
			authMode:         config.AuthModeToken,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode auto, two localhost addresses",
			listenAddrs:      config.ListenAddrs{Plain: "127.0.0.1:8080", TLS: "127.0.0.1:8081"},
			authMode:         config.AuthModeAuto,
			wantAuthRequired: false,
		},
		{
			name:             "no existing token, auth mode auto, one localhost address and one non-localhost address",
			listenAddrs:      config.ListenAddrs{Plain: "127.0.0.1:8080", TLS: ":8443"},
			authMode:         config.AuthModeAuto,
			wantAuthRequired: true,
		},
		{
			name:             "no existing token, auth mode auto, one localhost address and two non-localhost address",
			listenAddrs:      config.ListenAddrs{Plain: ":8080", TLS: ":8443"},
			authMode:         config.AuthModeAuto,
			wantAuthRequired: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			logger := testhelpers.NewTestLogger(t)
			cfg := config.Config{
				DataDir:             tempDir,
				ListenAddrs:         tc.listenAddrs,
				AuthMode:            tc.authMode,
				InsecureAllowNoAuth: tc.insecureAllowNoAuth,
			}

			if tc.existingToken != "" {
				tokenRecord, err := token.NewRecord(token.RawToken(tc.existingToken), time.Time{})
				require.NoError(t, err)

				tokenRecordBytes, err := json.Marshal(tokenRecord)
				require.NoError(t, err)

				require.NoError(t, os.WriteFile(filepath.Join(tempDir, "api-token.json"), tokenRecordBytes, 0600))
			}

			got, err := buildAPICredentials(cfg, logger)
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
