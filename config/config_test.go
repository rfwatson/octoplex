package config_test

import (
	"bytes"
	_ "embed"
	"io"
	"testing"

	"git.netflux.io/rob/octoplex/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/complete.yml
var configComplete []byte

//go:embed testdata/no-logfile.yml
var configNoLogfile []byte

//go:embed testdata/no-name.yml
var configNoName []byte

//go:embed testdata/invalid-destination-url.yml
var configInvalidDestinationURL []byte

//go:embed testdata/multiple-invalid-destination-urls.yml
var configMultipleInvalidDestinationURLs []byte

func TestConfig(t *testing.T) {
	testCases := []struct {
		name    string
		r       io.Reader
		want    func(*testing.T, config.Config)
		wantErr string
	}{
		{
			name: "complete",
			r:    bytes.NewReader(configComplete),
			want: func(t *testing.T, cfg config.Config) {
				require.Equal(
					t,
					config.Config{
						LogFile: "test.log",
						Destinations: []config.Destination{
							{
								Name: "my stream",
								URL:  "rtmp://rtmp.example.com:1935/live",
							},
						},
					}, cfg)
			},
		},
		{
			name: "no logfile",
			r:    bytes.NewReader(configNoLogfile),
			want: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "octoplex.log", cfg.LogFile)
			},
		},
		{
			name: "no name",
			r:    bytes.NewReader(configNoName),
			want: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "Stream 1", cfg.Destinations[0].Name)
			},
		},
		{
			name:    "invalid destination URL",
			r:       bytes.NewReader(configInvalidDestinationURL),
			wantErr: "destination URL must start with rtmp://",
		},
		{
			name:    "multiple invalid destination URLs",
			r:       bytes.NewReader(configMultipleInvalidDestinationURLs),
			wantErr: "destination URL must start with rtmp://\ndestination URL must start with rtmp://",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.Load(tc.r)

			if tc.wantErr == "" {
				require.NoError(t, err)
				tc.want(t, cfg)
			} else {
				assert.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func TestConfigDefault(t *testing.T) {
	cfg, err := config.Load(config.Default())
	require.NoError(t, err)
	assert.Equal(t, "octoplex.log", cfg.LogFile)
	assert.Empty(t, cfg.Destinations)
}
