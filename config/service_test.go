package config_test

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"

	"git.netflux.io/rob/octoplex/config"
	"git.netflux.io/rob/octoplex/shortid"
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

func TestConfigServiceCreateConfig(t *testing.T) {
	suffix := "read_or_create_" + shortid.New().String()
	service, err := config.NewService(configDirFunc(suffix))
	require.NoError(t, err)

	cfg, err := service.ReadOrCreateConfig()
	require.NoError(t, err)
	require.Equal(t, "octoplex.log", cfg.LogFile)

	p := filepath.Join(configDir(suffix), "config.yaml")
	_, err = os.Stat(p)
	require.NoError(t, err, "config file was not created")
}

func TestConfigServiceReadConfig(t *testing.T) {
	testCases := []struct {
		name        string
		configBytes []byte
		want        func(*testing.T, config.Config)
		wantErr     string
	}{
		{
			name:        "complete",
			configBytes: configComplete,
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
			name:        "no logfile",
			configBytes: configNoLogfile,
			want: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "octoplex.log", cfg.LogFile)
			},
		},
		{
			name:        "no name",
			configBytes: configNoName,
			want: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "Stream 1", cfg.Destinations[0].Name)
			},
		},
		{
			name:        "invalid destination URL",
			configBytes: configInvalidDestinationURL,
			wantErr:     "destination URL must start with rtmp://",
		},
		{
			name:        "multiple invalid destination URLs",
			configBytes: configMultipleInvalidDestinationURLs,
			wantErr:     "destination URL must start with rtmp://\ndestination URL must start with rtmp://",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			suffix := "read_or_create_" + shortid.New().String()
			dir := configDir(suffix)
			require.NoError(t, os.MkdirAll(dir, 0744))
			configPath := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, tc.configBytes, 0644))

			service, err := config.NewService(configDirFunc(suffix))
			require.NoError(t, err)

			cfg, err := service.ReadOrCreateConfig()

			if tc.wantErr == "" {
				require.NoError(t, err)
				tc.want(t, cfg)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func configDir(suffix string) string {
	return filepath.Join(os.TempDir(), "config_test_"+suffix, "octoplex")
}

func configDirFunc(suffix string) func() (string, error) {
	return func() (string, error) {
		return filepath.Join(os.TempDir(), "config_test_"+suffix), nil
	}
}
