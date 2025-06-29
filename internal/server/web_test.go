package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	mocks "git.netflux.io/rob/octoplex/internal/generated/mocks/server"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"git.netflux.io/rob/octoplex/internal/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWebHandlerSessionCreate(t *testing.T) {
	const password = "letmein123"

	testCases := []struct {
		name           string
		baseURL        string
		sentPassword   string
		tokenStoreFunc func(t *testing.T, tokenStore *mocks.TokenStore)
		wantStatus     int
		wantLocation   string
		wantCookie     bool
		wantSecure     bool
	}{
		{
			name:         "successful login, no base URL",
			sentPassword: password,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
				tokenStore.EXPECT().Put(storeKeySessionToken, mock.Anything).Return(nil)
			},
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/dashboard.html",
			wantCookie:   true,
			wantSecure:   true,
		},
		{
			name:         "successful login, HTTP base URL",
			baseURL:      "http://localhost:8080",
			sentPassword: password,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
				tokenStore.EXPECT().Put(storeKeySessionToken, mock.Anything).Return(nil)
			},
			wantStatus:   http.StatusSeeOther,
			wantLocation: "http://localhost:8080/dashboard.html",
			wantCookie:   true,
			wantSecure:   false,
		},
		{
			name:         "successful login, HTTPS base URL",
			baseURL:      "https://www.example.com",
			sentPassword: password,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
				tokenStore.EXPECT().Put(storeKeySessionToken, mock.Anything).Return(nil)
			},
			wantStatus:   http.StatusSeeOther,
			wantLocation: "https://www.example.com/dashboard.html",
			wantCookie:   true,
			wantSecure:   true,
		},
		{
			name:         "successful login, HTTPS base URL with a trailing slash",
			baseURL:      "https://www.example.com/",
			sentPassword: password,
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
				tokenStore.EXPECT().Put(storeKeySessionToken, mock.Anything).Return(nil)
			},
			wantStatus:   http.StatusSeeOther,
			wantLocation: "https://www.example.com/dashboard.html",
			wantCookie:   true,
			wantSecure:   true,
		},
		{
			name:         "wrong password",
			sentPassword: "nope",
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:         "empty password",
			sentPassword: "",
			tokenStoreFunc: func(t *testing.T, tokenStore *mocks.TokenStore) {
				token, err := token.New(token.RawToken(password), time.Time{})
				require.NoError(t, err)

				tokenStore.EXPECT().Get(storeKeyAdminPassword).Return(token, nil)
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		logger := testhelpers.NewTestLogger(t)

		t.Run(tc.name, func(t *testing.T) {
			values := url.Values{}
			values.Set("password", tc.sentPassword)

			req := httptest.NewRequestWithContext(t.Context(), "POST", "/session", strings.NewReader(values.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()

			var tokenStore mocks.TokenStore
			defer tokenStore.AssertExpectations(t)
			tc.tokenStoreFunc(t, &tokenStore)

			handler := webHandler(
				config.Config{
					ServerURL: config.ServerURL{BaseURL: tc.baseURL},
					Web:       config.Web{Enabled: true},
				},
				CredentialsModeEnabled,
				&tokenStore,
				logger,
			)

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantLocation, rec.Header().Get("Location"))

			if tc.wantCookie {
				cookie := rec.Result().Cookies()[0]
				assert.Equal(t, cookie.Name, cookieNameSession)
				assert.NotZero(t, cookie.Value)
				assert.Equal(t, "/", cookie.Path)
				assert.NotZero(t, cookie.Expires)
				assert.Less(t, cookie.Expires, time.Now().Add(25*time.Hour))
				assert.Equal(t, tc.wantSecure, cookie.Secure)
				assert.True(t, cookie.HttpOnly)
				assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
			} else {
				assert.Empty(t, rec.Result().Cookies(), "expected no cookies, but got one")
			}
		})
	}
}

func TestWebHandlerSessionDestroy(t *testing.T) {
	testCases := []struct {
		name         string
		baseURL      string
		wantLocation string
		wantSecure   bool
	}{
		{
			name:         "no base URL",
			wantLocation: "/",
			wantSecure:   true,
		},
		{
			name:         "http base URL",
			baseURL:      "http://localhost:8080",
			wantLocation: "http://localhost:8080/",
			wantSecure:   false,
		},
		{
			name:         "https base URL",
			baseURL:      "https://www.example.com",
			wantLocation: "https://www.example.com/",
			wantSecure:   true,
		},
		{
			name:         "https base URL with a trailing slash",
			baseURL:      "https://www.example.com/",
			wantLocation: "https://www.example.com/",
			wantSecure:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := testhelpers.NewTestLogger(t)
			dataDir := t.TempDir()

			var tokenStore mocks.TokenStore
			defer tokenStore.AssertExpectations(t)

			tokenStore.EXPECT().Delete(storeKeySessionToken).Return(nil)

			req := httptest.NewRequestWithContext(t.Context(), "POST", "/session/destroy", nil)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()

			handler := webHandler(
				config.Config{
					DataDir:   dataDir,
					ServerURL: config.ServerURL{BaseURL: tc.baseURL},
					Web:       config.Web{Enabled: true},
				},
				CredentialsModeEnabled,
				&tokenStore,
				logger,
			)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, tc.wantLocation, rec.Header().Get("Location"))

			cookie := rec.Result().Cookies()[0]
			assert.Equal(t, cookie.Name, cookieNameSession)
			assert.Empty(t, cookie.Value)
			assert.Equal(t, "/", cookie.Path)
			assert.Equal(t, time.Unix(0, 0).UTC(), cookie.Expires)
			assert.Equal(t, tc.wantSecure, cookie.Secure)
			assert.True(t, cookie.HttpOnly)
			assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
		})
	}
}
