package server

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/store"
	"git.netflux.io/rob/octoplex/internal/token"
)

const (
	storeKeyAPIToken      = "api-token"
	storeKeyAdminPassword = "admin-password"
	storeKeySessionToken  = "session-token"
)

// CredentialsMode represents the mode of credentials used by the server.
//
// This is related to config.AuthMode, but represents the actual state of the
// server rather than the user's stated preference.
type CredentialsMode int

const (
	CredentialsModeEnabled CredentialsMode = iota
	CredentialsModeDisabled
)

// TokenStore is a store for tokens.
type TokenStore interface {
	Get(key string) (domain.Token, error)
	Put(key string, value domain.Token) error
	Delete(key string) error
}

// initCredentials initializes the credentials for the server based on the
// provided configuration. If the configuration requires authentication, an API
// token will be generated and stored securely. If the web interface is
// enabled, an admin password will also be generated and stored.
//
// It returns an indication of whether credentials are enabled or disabled.
func initCredentials(cfg config.Config, tokenStore TokenStore, logger *slog.Logger) (_ CredentialsMode, err error) {
	disabled, err := initAPICredentials(cfg, tokenStore, logger)
	if err != nil {
		return CredentialsModeEnabled, fmt.Errorf("build API credentials: %w", err)
	} else if disabled {
		return CredentialsModeDisabled, nil
	}

	if !cfg.Web.Enabled {
		return CredentialsModeEnabled, nil
	}

	if err = initAdminPassword(tokenStore, logger); err != nil {
		return CredentialsModeEnabled, fmt.Errorf("load or generate admin password: %w", err)
	}

	return CredentialsModeEnabled, nil
}

func initAPICredentials(cfg config.Config, tokenStore TokenStore, logger *slog.Logger) (disabled bool, err error) {
	if cfg.ListenAddrs.Plain == "" && cfg.ListenAddrs.TLS == "" {
		return false, fmt.Errorf("listen addresses cannot all be empty")
	}

	isLoopback, err := isLoopback(cfg)
	if err != nil {
		return false, fmt.Errorf("check loopback: %w", err)
	}

	var tokenExists bool
	_, err = tokenStore.Get(storeKeyAPIToken)
	if err != nil {
		if err != store.ErrTokenNotFound {
			return false, fmt.Errorf("read token: %w", err)
		}
	} else {
		tokenExists = true
	}

	// If auth mode is set to none, and insecure allow no auth is set, and it's a
	// loopback address - allow no authentication regardless of whether a token
	// exists. This is mostly for all-in-one mode.
	if cfg.AuthMode == config.AuthModeNone && cfg.InsecureAllowNoAuth && isLoopback {
		return true, nil
	}

	// Next, if a token exists, we always use it, regardless of the requested mode.
	if tokenExists {
		logger.Info("Enabling API authentication due to existing token")
		return false, nil
	}

	// Next handle auth mode none, which is not allowed for non-loopback
	// addresses unless explicitly enabled.
	if cfg.AuthMode == config.AuthModeNone {
		if !isLoopback && !cfg.InsecureAllowNoAuth {
			return false, ErrAuthenticationCannotBeDisabled
		}

		logger.Warn("WARNING: API authentication disabled. This is not recommended for production use.")
		return true, nil
	}

	// If the listen address is a loopback address, and auth mode is set to auto, we disable authentication
	if cfg.AuthMode == config.AuthModeAuto && isLoopback {
		logger.Info("No authentication required when only listening on loopback addresses")
		return true, nil
	}

	// Otherwise, generate a new token and require it.
	rawAPIToken, err := generateAPIToken(tokenStore)
	if err != nil {
		return false, fmt.Errorf("write API token: %w", err)
	}

	logger.Info(fmt.Sprintf("New API token generated. Store it now - it will not be shown again. TOKEN: %s", hex.EncodeToString(rawAPIToken)))
	return false, nil
}

func generateAPIToken(tokenStore TokenStore) (token.RawToken, error) {
	const tokenLenBytes = 32 // length of raw token in bytes
	rawToken, err := token.GenerateRawToken(tokenLenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate raw token: %w", err)
	}

	apiToken, err := token.New(rawToken, time.Time{}) // for now, no expiry
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	if err := tokenStore.Put(storeKeyAPIToken, apiToken); err != nil {
		return nil, fmt.Errorf("write token: %w", err)
	}

	return rawToken, nil
}

func initAdminPassword(tokenStore TokenStore, logger *slog.Logger) error {
	if _, err := tokenStore.Get(storeKeyAdminPassword); err != nil {
		if err != store.ErrTokenNotFound {
			return fmt.Errorf("read admin password record: %w", err)
		}
	} else {
		return nil
	}

	rawPassword, err := generateAdminPassword(tokenStore)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}

	logger.Info(fmt.Sprintf("New admin password generated. Store it now - it will not be shown again. ADMIN PASSWORD: %s", rawPassword))
	return nil
}

func generateAdminPassword(tokenStore TokenStore) (token.RawToken, error) {
	const passwordLenBytes = 8 // length of raw password in bytes
	rawPasswordBytes, err := token.GenerateRawToken(passwordLenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate raw token: %w", err)
	}
	rawPassword := token.RawToken(base64.RawStdEncoding.EncodeToString(rawPasswordBytes))

	adminPassword, err := token.New(rawPassword, time.Time{}) // passwords do not expire
	if err != nil {
		return nil, fmt.Errorf("create token: %w", err)
	}

	if err := tokenStore.Put(storeKeyAdminPassword, adminPassword); err != nil {
		return nil, fmt.Errorf("write token: %w", err)
	}

	return rawPassword, nil
}

func generateSessionToken(validFor time.Duration, tokenStore TokenStore) (string, error) {
	const lenBytes = 64 // length of raw session token in bytes
	rawToken, err := token.GenerateRawToken(lenBytes)
	if err != nil {
		return "", fmt.Errorf("generate raw token: %w", err)
	}

	sessionToken, err := token.New(rawToken, time.Now().Add(validFor))
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}

	if err := tokenStore.Put(storeKeySessionToken, sessionToken); err != nil {
		return "", fmt.Errorf("write token: %w", err)
	}

	return hex.EncodeToString(rawToken), nil
}

func isLoopback(cfg config.Config) (bool, error) {
	var isLoopbacks []bool

	if cfg.ListenAddrs.Plain != "" {
		addr, err := net.ResolveTCPAddr("tcp", cfg.ListenAddrs.Plain)
		if err != nil {
			return false, fmt.Errorf("resolve TLS: %w", err)
		}

		isLoopbacks = append(isLoopbacks, addr.IP.IsLoopback())
	}

	if cfg.ListenAddrs.TLS != "" {
		addr, err := net.ResolveTCPAddr("tcp", cfg.ListenAddrs.TLS)
		if err != nil {
			return false, fmt.Errorf("resolve plain: %w", err)
		}

		isLoopbacks = append(isLoopbacks, addr.IP.IsLoopback())
	}

	for _, isLoopback := range isLoopbacks {
		if !isLoopback {
			return false, nil
		}
	}
	return true, nil
}

// isAuthenticatedWithAPIToken checks if the request is authenticated using the provided
// API credentials. It returns true, nil if the request is authenticated. If
// there is an error then it is responsible for logging it.
func isAuthenticatedWithAPIToken(authHeader string, tokenStore TokenStore, logger *slog.Logger) bool {
	if authHeader == "" {
		return false
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	rawToken, err := hex.DecodeString(strings.TrimPrefix(authHeader, "Bearer "))
	if err != nil {
		return false
	}

	apiToken, err := tokenStore.Get(storeKeyAPIToken)
	if err != nil {
		logger.Error("Error retrieving API token from store", "err", err)
		return false
	}

	if isValid, err := token.Matches(apiToken, token.RawToken(rawToken)); err != nil || !isValid {
		if err != nil {
			logger.Error("Error authenticating", "err", err)
		}

		return false
	}

	return true
}

func isAuthenticatedWithSessionToken(cookieHeader string, tokenStore TokenStore, logger *slog.Logger) bool {
	if cookieHeader == "" {
		return false
	}

	cookies, err := http.ParseCookie(cookieHeader)
	if err != nil {
		return false
	}

	var cookie *http.Cookie
	for _, c := range cookies {
		if c.Name == cookieNameSession {
			cookie = c
			break
		}
	}
	if cookie == nil {
		return false
	}

	rawToken, err := hex.DecodeString(cookie.Value)
	if err != nil {
		return false
	}

	sessionToken, err := tokenStore.Get(storeKeySessionToken)
	if err != nil {
		logger.Error("Error retrieving session token from store", "err", err)
		return false
	}

	if isValid, err := token.Matches(sessionToken, token.RawToken(rawToken)); err != nil || !isValid {
		if err != nil {
			logger.Error("Error authenticating", "err", err)
		}

		return false
	}

	return true
}

// ResetCredentials resets the API token and the admin password. It returns the
// raw token strings that can be presented to the user.
func ResetCredentials(cfg config.Config, tokenStore TokenStore) (string, string, error) {
	if err := tokenStore.Delete(storeKeyAPIToken); err != nil && !errors.Is(err, store.ErrTokenNotFound) {
		return "", "", fmt.Errorf("delete API token: %w", err)
	}

	if err := tokenStore.Delete(storeKeyAdminPassword); err != nil && !errors.Is(err, store.ErrTokenNotFound) {
		return "", "", fmt.Errorf("delete admin password: %w", err)
	}

	if err := tokenStore.Delete(storeKeySessionToken); err != nil && !errors.Is(err, store.ErrTokenNotFound) {
		return "", "", fmt.Errorf("delete session token: %w", err)
	}

	apiToken, err := generateAPIToken(tokenStore)
	if err != nil {
		return "", "", fmt.Errorf("generate API token: %w", err)
	}

	adminPassword, err := generateAdminPassword(tokenStore)
	if err != nil {
		return "", "", fmt.Errorf("generate admin password: %w", err)
	}

	return hex.EncodeToString(apiToken), string(adminPassword), nil
}
