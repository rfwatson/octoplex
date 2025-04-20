package domain_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestAppStateClone(t *testing.T) {
	s := &domain.AppState{
		Source: domain.Source{Live: true},
		Destinations: []domain.Destination{
			{
				Container: domain.Container{ID: "123"},
				Status:    0,
				Name:      "YouTube",
				URL:       "rtmp://a.rtmp.youtube.com/live2",
			},
		},
		BuildInfo: domain.BuildInfo{Version: "1.0.0"},
	}

	s2 := s.Clone()

	assert.Equal(t, s.Source.Live, s2.Source.Live)
	assert.Equal(t, s.Destinations, s2.Destinations)
	assert.Equal(t, s.BuildInfo, s2.BuildInfo)

	// ensure the destinations slice is cloned
	s.Destinations[0].Name = "Twitch"
	assert.Equal(t, "YouTube", s2.Destinations[0].Name)
}

func TestNetAddr(t *testing.T) {
	var addr domain.NetAddr
	assert.True(t, addr.IsZero())

	addr.IP = "127.0.0.1"
	addr.Port = 3000
	assert.False(t, addr.IsZero())
}

func TestKeyPair(t *testing.T) {
	var keyPair domain.KeyPair
	assert.True(t, keyPair.IsZero())

	keyPair.Cert = []byte("cert")
	keyPair.Key = []byte("key")
	assert.False(t, keyPair.IsZero())
}
