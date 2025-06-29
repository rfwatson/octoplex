package server

import (
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
	cookieValidFor    = 24 * time.Hour
)

// TODO: improve error handling
func webHandler(cfg config.Config, credentialsMode CredentialsMode, tokenStore TokenStore, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	if credentialsMode != CredentialsModeDisabled {
		mux.Handle("POST /session", handleSessionCreate(cfg, tokenStore, logger))
		mux.Handle("POST /session/destroy", handleSessionDestroy(cfg, tokenStore, logger))
	}

	return mux
}

func handleSessionCreate(cfg config.Config, tokenStore TokenStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminPassword, err := tokenStore.Get(storeKeyAdminPassword)
		if err != nil {
			logger.Error("Error retrieving admin password from store", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error")) //nolint:errcheck
			return
		}

		matches, err := token.Matches(adminPassword, []byte(r.FormValue("password")))
		if err != nil {
			logger.Error("Error comparing password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error")) //nolint:errcheck
			return
		}

		if !matches {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized")) //nolint:errcheck
			return
		}

		sessionToken, err := generateSessionToken(cookieValidFor-time.Minute, tokenStore)
		if err != nil {
			logger.Error("Error generating session token", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error")) //nolint:errcheck
			return
		}

		setSessionCookie(w, sessionToken, cfg.ServerURL.BaseURL)
		w.Header().Set("Location", joinURLComponents(cfg.ServerURL.BaseURL, "/dashboard.html"))
		w.WriteHeader(http.StatusSeeOther)
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
		w.Header().Set("Location", joinURLComponents(cfg.ServerURL.BaseURL, "/"))
		w.WriteHeader(http.StatusSeeOther)
	}
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
