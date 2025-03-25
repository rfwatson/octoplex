package config_test

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/shortid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/complete.yml
var configComplete []byte

//go:embed testdata/no-logfile.yml
var configNoLogfile []byte

//go:embed testdata/logfile.yml
var configLogfile []byte

//go:embed testdata/no-name.yml
var configNoName []byte

//go:embed testdata/invalid-destination-url.yml
var configInvalidDestinationURL []byte

//go:embed testdata/multiple-invalid-destination-urls.yml
var configMultipleInvalidDestinationURLs []byte

func TestConfigServiceCreateConfig(t *testing.T) {
	suffix := "read_or_create_" + shortid.New().String()
	systemConfigDirFunc := buildSystemConfigDirFunc(suffix)
	systemConfigDir, _ := systemConfigDirFunc()

	service, err := config.NewService(systemConfigDirFunc)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, os.RemoveAll(systemConfigDir)) })

	cfg, err := service.ReadOrCreateConfig()
	require.NoError(t, err)
	require.Empty(t, cfg.LogFile, "expected no log file")

	p := filepath.Join(systemConfigDir, "octoplex", "config.yaml")
	cfgBytes, err := os.ReadFile(p)
	require.NoError(t, err, "config file was not created")
	// Ensure the example config file is written:
	assert.Contains(t, string(cfgBytes), "# Octoplex is a live stream multiplexer.")
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
						LogFile: config.LogFile{
							Enabled: true,
							Path:    "test.log",
						},
						Sources: config.Sources{
							RTMP: config.RTMPSource{
								Enabled:   true,
								StreamKey: "s3cr3t",
							},
						},
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
			name:        "logging enabled, no logfile",
			configBytes: configNoLogfile,
			want: func(t *testing.T, cfg config.Config) {
				assert.True(t, strings.HasSuffix(cfg.LogFile.Path, "/octoplex/octoplex.log"))
			},
		},
		{
			name:        "logging enabled, logfile",
			configBytes: configLogfile,
			want: func(t *testing.T, cfg config.Config) {
				assert.Equal(t, "/tmp/octoplex.log", cfg.LogFile.Path)
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
			systemConfigDirFunc := buildSystemConfigDirFunc(suffix)
			systemConfigDir, _ := systemConfigDirFunc()
			appConfigDir := buildAppConfigDir(suffix)

			require.NoError(t, os.MkdirAll(appConfigDir, 0744))
			t.Cleanup(func() { require.NoError(t, os.RemoveAll(systemConfigDir)) })

			configPath := filepath.Join(appConfigDir, "config.yaml")
			require.NoError(t, os.WriteFile(configPath, tc.configBytes, 0644))

			service, err := config.NewService(buildSystemConfigDirFunc(suffix))
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

// buildAppConfigDir returns a temporary directory which mimics
// $XDG_CONFIG_HOME/octoplex.
func buildAppConfigDir(suffix string) string {
	return filepath.Join(os.TempDir(), "config_test_"+suffix, "octoplex")
}

// buildSystemConfigDirFunc returns a function that creates a temporary
// directory which mimics $XDG_CONFIG_HOME.
func buildSystemConfigDirFunc(suffix string) func() (string, error) {
	return func() (string, error) {
		return filepath.Join(os.TempDir(), "config_test_"+suffix), nil
	}
}
