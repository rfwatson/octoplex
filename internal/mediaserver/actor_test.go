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
