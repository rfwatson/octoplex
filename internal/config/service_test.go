package config_test

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/shortid"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

//go:embed testdata/complete.yml
var configComplete []byte

//go:embed testdata/logfile.yml
var configLogfile []byte

//go:embed testdata/no-logfile.yml
var configNoLogfile []byte

//go:embed testdata/destination-url-not-rtmp.yml
var configDestinationURLNotRTMP []byte

//go:embed testdata/destination-url-not-valid.yml
var configDestinationURLNotValid []byte

//go:embed testdata/multiple-invalid-destination-urls.yml
var configMultipleInvalidDestinationURLs []byte

func TestConfigServiceCurrent(t *testing.T) {
	suffix := "current_" + shortid.New().String()
	systemConfigDirFunc := buildSystemConfigDirFunc(suffix)
	systemConfigDir, _ := systemConfigDirFunc()

	service, err := config.NewService(systemConfigDirFunc, 1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(systemConfigDir)) })

	// Ensure defaults are set:
	assert.True(t, service.Current().Sources.RTMP.Enabled)
}

func TestConfigServiceCreateConfig(t *testing.T) {
	suffix := "read_or_create_" + shortid.New().String()
	systemConfigDirFunc := buildSystemConfigDirFunc(suffix)
	systemConfigDir, _ := systemConfigDirFunc()

	service, err := config.NewService(systemConfigDirFunc, 1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(systemConfigDir)) })

	cfg, err := service.ReadOrCreateConfig()
	require.NoError(t, err)
	require.False(t, cfg.LogFile.Enabled, "expected logging to be disabled")
	require.Empty(t, cfg.LogFile.Path, "expected no log file")

	p := filepath.Join(systemConfigDir, "octoplex", "config.yaml")
	cfgBytes, err := os.ReadFile(p)
	require.NoError(t, err, "config file was not created")

	var readCfg config.Config
	require.NoError(t, yaml.Unmarshal(cfgBytes, &readCfg))
	assert.True(t, readCfg.Sources.RTMP.Enabled, "default values not set")
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
				require.Empty(
					t,
					gocmp.Diff(
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
						},
						cfg,
						cmpopts.IgnoreUnexported(config.LogFile{}),
					),
				)
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
			name:        "logging enabled, no logfile",
			configBytes: configNoLogfile,
			want: func(t *testing.T, cfg config.Config) {
				assert.True(t, strings.HasSuffix(cfg.LogFile.GetPath(), "/octoplex/octoplex.log"), "expected %q to end with /tmp/octoplex.log", cfg.LogFile.GetPath())
			},
		},
		{
			name:        "destination URL is not rtmp scheme",
			configBytes: configDestinationURLNotRTMP,
			wantErr:     "destination URL must be an RTMP URL",
		},
		{
			name:        "destination URL is not valid",
			configBytes: configDestinationURLNotValid,
			wantErr:     `invalid destination URL: parse "rtmp://rtmp.example.com/%%+": invalid URL escape "%%+"`,
		},
		{
			name:        "multiple invalid destination URLs",
			configBytes: configMultipleInvalidDestinationURLs,
			wantErr:     "destination URL must be an RTMP URL\ndestination URL must be an RTMP URL",
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

			service, err := config.NewService(buildSystemConfigDirFunc(suffix), 1)
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

func TestConfigServiceSetConfig(t *testing.T) {
	suffix := "set_config_" + shortid.New().String()
	systemConfigDirFunc := buildSystemConfigDirFunc(suffix)
	systemConfigDir, _ := systemConfigDirFunc()

	service, err := config.NewService(systemConfigDirFunc, 1)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(systemConfigDir)) })

	cfg := config.Config{LogFile: config.LogFile{Enabled: true, Path: "test.log"}}
	require.NoError(t, service.SetConfig(cfg))

	cfg, err = service.ReadOrCreateConfig()
	require.NoError(t, err)

	assert.Equal(t, "test.log", cfg.LogFile.Path)
	assert.True(t, cfg.LogFile.Enabled)
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
