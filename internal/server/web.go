package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/token"
)

const (
	cookiePath        = "/"
	cookieNameSession = "octoplex-session"
	cookieValidFor    = 7 * 24 * time.Hour
)

// TODO: improve error responses
func newWebHandler(cfg config.Config, internalAPI *Server, credentialsMode CredentialsMode, tokenStore TokenStore, logger *slog.Logger) (http.Handler, error) {
	mux := http.NewServeMux()

	mux.Handle("POST /session", handleSessionCreate(cfg, credentialsMode, tokenStore, logger))
	mux.Handle("POST /session/destroy", handleSessionDestroy(cfg, tokenStore, logger))
	mux.Handle("POST /session/refresh", handleSessionRefresh(cfg, credentialsMode, tokenStore, logger))
	mux.Handle("/ws", newWebSocketProxy(cfg, internalAPI, credentialsMode, tokenStore, logger).Handler())

	if serveAssets {
		subFS, err := fs.Sub(assetsFS, "assets")
		if err != nil {
			return nil, fmt.Errorf("sub FS: %w", err)
		}

		mux.Handle("/", http.FileServer(http.FS(subFS)))
	} else {
		logger.Info("Serving assets is disabled, no static files will be served")
	}

	return defaultHeaders(mux), nil
}

func handleSessionCreate(cfg config.Config, credentialsMode CredentialsMode, tokenStore TokenStore, logger *slog.Logger) http.HandlerFunc {
	type request struct {
		Password string `json:"password"`
	}

	type response struct {
		Success  bool   `json:"success"`
		Error    string `json:"error,omitzero"`
		Field    string `json:"field,omitzero"`
		Redirect string `json:"redirect,omitzero"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if credentialsMode == CredentialsModeDisabled {
			logger.Warn("Session creation attempted in disabled credentials mode", "remote_addr", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Authentication disabled"}) //nolint:errcheck
			return
		}

		adminPassword, err := tokenStore.Get(storeKeyAdminPassword)
		if err != nil {
			logger.Error("Error retrieving admin password from store", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Internal server error"}) //nolint:errcheck
			return
		}

		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Error reading request body", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Invalid request body"}) //nolint:errcheck
			return
		}

		var req request
		if err = json.Unmarshal(reqBody, &req); err != nil {
			logger.Error("Error parsing request body", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Error parsing request body"}) //nolint:errcheck
			return
		}

		matches, err := token.Matches(adminPassword, []byte(req.Password))
		if err != nil {
			logger.Error("Error comparing password", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Internal server error"}) //nolint:errcheck
			return
		}

		if !matches {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Invalid password", Field: "password"}) //nolint:errcheck
			return
		}

		sessionToken, err := generateSessionToken(cookieValidFor-time.Minute, tokenStore)
		if err != nil {
			logger.Error("Error generating session token", "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response{Success: false, Error: "Internal server error"}) //nolint:errcheck
			return
		}

		setSessionCookie(w, sessionToken, cfg.ServerURL.BaseURL)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response{Success: true, Redirect: "/"}) //nolint:errcheck
	}
}

func handleSessionDestroy(cfg config.Config, tokenStore TokenStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tokenStore.Delete(storeKeySessionToken); err != nil {
			logger.Error("Error deleting session token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error")) //nolint:errcheck
			return
		}
		setSessionCookie(w, "", cfg.ServerURL.BaseURL)
		w.Header().Set("Location", joinURLComponents(cfg.ServerURL.BaseURL, "/login.html"))
		w.WriteHeader(http.StatusSeeOther)
	}
}

func handleSessionRefresh(cfg config.Config, credentialsMode CredentialsMode, tokenStore TokenStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if credentialsMode == CredentialsModeDisabled {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if !isAuthenticatedWithSessionToken(r.Header.Get("cookie"), tokenStore, logger) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := refreshSessionToken(tokenStore, logger); err != nil {
			logger.Error("Error refreshing session token", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		sessionTokenCookie, err := r.Cookie(cookieNameSession)
		if err != nil {
			logger.Warn("Session token cookie not found", "err", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		setSessionCookie(w, sessionTokenCookie.Value, cfg.ServerURL.BaseURL)

		w.WriteHeader(http.StatusNoContent)
	}
}

func defaultHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-content-type-options", "nosniff")
		w.Header().Set("x-frame-options", "DENY")
		w.Header().Set("referrer-policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func setSessionCookie(w http.ResponseWriter, sessionToken string, baseURL string) {
	// Secure cookies by default, unless the base URL is explicitly set to HTTP.
	//
	// TODO: consider exposing secure cookie configuration as an explicit config
	// option.
	secure := !strings.HasPrefix(baseURL, "http://")

	var expires time.Time
	if sessionToken == "" {
		expires = time.Unix(0, 0).UTC()
	} else {
		expires = time.Now().Add(cookieValidFor)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieNameSession,
		Value:    sessionToken,
		Path:     cookiePath,
		Expires:  expires,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func joinURLComponents(hostPart, path string) string {
	var s strings.Builder

	s.WriteString(hostPart)
	if !strings.HasSuffix(hostPart, "/") {
		s.WriteString("/")
	}
	s.WriteString(strings.TrimPrefix(path, "/"))

	return s.String()
}
