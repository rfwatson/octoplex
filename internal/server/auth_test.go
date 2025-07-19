package server

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	mocks "git.netflux.io/rob/octoplex/internal/generated/mocks/server"
	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInitCredentials(t *testing.T) {
	testCases := []struct {
		name                string
		listenAddrs         config.ListenAddrs
		authMode            config.AuthMode
		webEnabled          bool
		insecureAllowNoAuth bool
		tokenStoreFunc      func(t *testing.T, tokeStore *mocks.TokenStore)
		wantCredentialsMode CredentialsMode
		wantErr             error
	}{
		{
			name:        "existing token, auth mode token",
			listenAddrs: config.ListenAddrs{TLS: ":8443"},
			authMode:    config.AuthModeToken,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{Hashed: "abcdef"}, nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "existing token, auth mode none",
			listenAddrs: config.ListenAddrs{TLS: ":8443"},
			authMode:    config.AuthModeNone,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{Hashed: "abcdef"}, nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:                "existing token, auth mode none, localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{Hashed: "abcdef"}, nil)
			},
			wantCredentialsMode: CredentialsModeDisabled,
		},
		{
			name:                "existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: ":8443"},
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{Hashed: "abcdef"}, nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "existing token, auth mode token, web enabled",
			listenAddrs: config.ListenAddrs{TLS: ":8443"},
			authMode:    config.AuthModeToken,
			webEnabled:  true,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{Hashed: "abcdef"}, nil)
				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(domain.Token{Hashed: "adminpassword"}, nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode none, localhost, no insecure allow no auth",
			listenAddrs: config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:    config.AuthModeNone,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
			},
			wantCredentialsMode: CredentialsModeDisabled,
		},
		{
			name:        "no existing token, auth mode none, non-localhost, no insecure allow no auth",
			listenAddrs: config.ListenAddrs{TLS: "0.0.0.0:8443"},
			authMode:    config.AuthModeNone,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
			},
			wantErr: ErrAuthenticationCannotBeDisabled,
		},
		{
			name:                "no existing token, auth mode none, non-localhost, insecure allow no auth",
			listenAddrs:         config.ListenAddrs{TLS: ":8443"},
			authMode:            config.AuthModeNone,
			insecureAllowNoAuth: true,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
			},
			wantCredentialsMode: CredentialsModeDisabled,
		},
		{
			name:        "no existing token, auth mode auto, localhost address",
			listenAddrs: config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:    config.AuthModeAuto,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
			},
			wantCredentialsMode: CredentialsModeDisabled,
		},
		{
			name:        "no existing token, auth mode auto, non-localhost address",
			listenAddrs: config.ListenAddrs{TLS: "192.168.1.100:8443"},
			authMode:    config.AuthModeAuto,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode token, localhost address",
			listenAddrs: config.ListenAddrs{TLS: "127.0.0.1:8443"},
			authMode:    config.AuthModeToken,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode token, non-localhost address",
			listenAddrs: config.ListenAddrs{TLS: "192.168.1.100:8443"},
			authMode:    config.AuthModeToken,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode token, non-localhost address, web enabled",
			listenAddrs: config.ListenAddrs{TLS: "192.168.1.100:8443"},
			authMode:    config.AuthModeToken,
			webEnabled:  true,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(domain.Token{Hashed: "adminpassword"}, nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode auto, two localhost addresses",
			listenAddrs: config.ListenAddrs{Plain: "127.0.0.1:8080", TLS: "127.0.0.1:8081"},
			authMode:    config.AuthModeAuto,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
			},
			wantCredentialsMode: CredentialsModeDisabled,
		},
		{
			name:        "no existing token, auth mode auto, one localhost address and one non-localhost address",
			listenAddrs: config.ListenAddrs{Plain: "127.0.0.1:8080", TLS: ":8443"},
			authMode:    config.AuthModeAuto,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
		{
			name:        "no existing token, auth mode auto, one localhost address and two non-localhost address",
			listenAddrs: config.ListenAddrs{Plain: ":8080", TLS: ":8443"},
			authMode:    config.AuthModeAuto,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				tokenStore.EXPECT().Get(storeKeyAPIToken).Return(domain.Token{}, store.ErrTokenNotFound)
				tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
			},
			wantCredentialsMode: CredentialsModeEnabled,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			logger := testhelpers.NewTestLogger(t)
			cfg := config.Config{
				DataDir:             dataDir,
				ListenAddrs:         tc.listenAddrs,
				AuthMode:            tc.authMode,
				InsecureAllowNoAuth: tc.insecureAllowNoAuth,
				Web:                 config.Web{Enabled: tc.webEnabled},
			}

			var tokenStore mocks.TokenStore
			defer tokenStore.AssertExpectations(t)
			tc.tokenStoreFunc(t, &tokenStore)

			credentialsMode, err := initCredentials(cfg, &tokenStore, logger)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantCredentialsMode, credentialsMode)
			}
		})
	}
}

func TestResetCredentials(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{
		DataDir: dataDir,
	}
	var tokenStore mocks.TokenStore
	defer tokenStore.AssertExpectations(t)
	tokenStore.EXPECT().Delete(storeKeyAPIToken).Return(nil)
	tokenStore.EXPECT().Delete(storeKeyAdminPassword).Return(nil)
	tokenStore.EXPECT().Delete(storeKeySessionToken).Return(nil)
	tokenStore.EXPECT().Put(storeKeyAPIToken, mock.Anything).Return(nil)
	tokenStore.EXPECT().Put(storeKeyAdminPassword, mock.Anything).Return(nil)

	apiToken, adminPassword, err := ResetCredentials(cfg, &tokenStore)
	require.NoError(t, err)
	assert.NotEmpty(t, apiToken)
	assert.NotEmpty(t, adminPassword)
}
