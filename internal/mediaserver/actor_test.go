package mediaserver

import (
	"testing"

	mocks "git.netflux.io/rob/octoplex/internal/generated/mocks/mediaserver"
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
		name             string
		dockerHost       string
		inDocker         bool
		hasDockerNetwork bool
		want             string
	}{
		{
			name:       "not in docker",
			dockerHost: "localhost",
			inDocker:   false,
			want:       "https://api:s3cr3t@localhost:9997/v3/paths/get/live",
		},
		{
			name:             "in docker, not connected to Octoplex network",
			dockerHost:       "localhost",
			inDocker:         true,
			hasDockerNetwork: false,
			want:             "https://api:s3cr3t@localhost:9997/v3/paths/get/live",
		},
		{
			name:             "in docker, not connected to Octoplex network, custom Docker host",
			dockerHost:       "my.docker.com",
			inDocker:         true,
			hasDockerNetwork: false,
			want:             "https://api:s3cr3t@my.docker.com:9997/v3/paths/get/live",
		},
		{
			name:             "in docker, connected to Octoplex network",
			dockerHost:       "localhost",
			inDocker:         true,
			hasDockerNetwork: true,
			want:             "https://api:s3cr3t@mediaserver:9997/v3/paths/get/live",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var containerClient mocks.ContainerClient
			defer containerClient.AssertExpectations(t)
			containerClient.EXPECT().HasDockerNetwork().Return(tc.hasDockerNetwork)

			actor := &Actor{
				apiPort:         9997,
				pass:            pass,
				dockerHost:      tc.dockerHost,
				inDocker:        tc.inDocker,
				containerClient: &containerClient,
			}

			assert.Equal(t, tc.want, actor.pathURL(path))
		})
	}
}
