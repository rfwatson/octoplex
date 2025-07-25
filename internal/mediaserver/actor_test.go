package mediaserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGeneratePassword(t *testing.T) {
	assert.Len(t, generatePassword(), 64)
	assert.NotEqual(t, generatePassword(), generatePassword())
}

func TestBuildServerConfig(t *testing.T) {
	var actor Actor

	bytes, err := actor.buildServerConfig()
	require.NoError(t, err)

	var cfg Config
	require.NoError(t, yaml.Unmarshal(bytes, &cfg))

	assert.Equal(t, "info", cfg.LogLevel) // Important: avoid leaking credentials
	assert.True(t, cfg.RTMP)
	assert.Equal(t, ":1935", cfg.RTMPAddress)
	assert.Equal(t, ":1936", cfg.RTMPSAddress)
	assert.True(t, cfg.API)
	assert.True(t, cfg.APIEncryption)
	assert.Equal(t, ":9997", cfg.APIAddress)
	assert.Equal(t, "internal", cfg.AuthMethod)
}

func TestPathURL(t *testing.T) {
	const (
		pass = "s3cr3t"
		path = "live"
	)

	testCases := []struct {
		name     string
		inDocker bool
		want     string
	}{
		{
			name:     "not in docker",
			inDocker: false,
			want:     "https://api:s3cr3t@localhost:9997/v3/paths/get/live",
		},
		{
			name:     "in docker",
			inDocker: true,
			want:     "https://api:s3cr3t@mediaserver:9997/v3/paths/get/live",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actor := &Actor{
				apiPort:  9997,
				pass:     pass,
				inDocker: tc.inDocker,
			}

			assert.Equal(t, tc.want, actor.pathURL(path))
		})
	}
}
