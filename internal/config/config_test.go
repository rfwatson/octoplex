package config_test

import (
	"errors"
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerAddress(t *testing.T) {
	testCases := []struct {
		name    string
		raw     string
		want    config.ServerURL
		wantErr error
	}{
		{
			name:    "empty raw string",
			wantErr: errors.New("empty URL"),
		},
		{
			name:    "invalid URL",
			raw:     "http://[::1",
			wantErr: errors.New("invalid server URL: parse \"http://[::1\": missing ']' in host"),
		},
		{
			name:    "URL missing host",
			raw:     "https:///",
			wantErr: errors.New("invalid server URL: no hostname found"),
		},
		{
			name:    "URL missing scheme",
			raw:     "://www.mydomain.com:8080",
			wantErr: errors.New("invalid server URL: parse \"://www.mydomain.com:8080\": missing protocol scheme"),
		},
		{
			name:    "URL invalid scheme",
			raw:     "ftp://www.mydomain.com:8080",
			wantErr: errors.New("invalid server URL: unsupported or missing scheme (must be http or https)"),
		},
		{
			name: "valid HTTP URL",
			raw:  "http://localhost:8080",
			want: config.ServerURL{
				Raw:      "http://localhost:8080",
				Hostname: "localhost",
				Port:     "8080",
				Scheme:   "http",
				BaseURL:  "http://localhost:8080/",
			},
		},
		{
			name: "valid HTTPS URL",
			raw:  "https://localhost:8080",
			want: config.ServerURL{
				Raw:      "https://localhost:8080",
				Hostname: "localhost",
				Port:     "8080",
				Scheme:   "https",
				BaseURL:  "https://localhost:8080/",
			},
		},
		{
			name: "HTTPS URL without a port",
			raw:  "https://www.mydomain.com",
			want: config.ServerURL{
				Raw:      "https://www.mydomain.com",
				Hostname: "www.mydomain.com",
				Scheme:   "https",
				BaseURL:  "https://www.mydomain.com/",
			},
		},
		{
			name: "URL with a trailing slash",
			raw:  "https://www.mydomain.com/",
			want: config.ServerURL{
				Raw:      "https://www.mydomain.com/",
				Hostname: "www.mydomain.com",
				Scheme:   "https",
				BaseURL:  "https://www.mydomain.com/",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			serverAddr, err := config.NewServerURL(tc.raw)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, serverAddr)
			}
		})
	}
}
