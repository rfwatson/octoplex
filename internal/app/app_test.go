package app

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestResolveDestinations(t *testing.T) {
	testCases := []struct {
		name     string
		in       []config.Destination
		existing []domain.Destination
		want     []domain.Destination
	}{
		{
			name:     "nil slices",
			existing: nil,
			want:     nil,
		},
		{
			name:     "empty slices",
			existing: []domain.Destination{},
			want:     []domain.Destination{},
		},
		{
			name:     "identical slices",
			in:       []config.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
			want:     []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
		},
		{
			name:     "adding a new destination",
			in:       []config.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}},
			want:     []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}},
		},
		{
			name:     "removing a destination",
			in:       []config.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}},
			want:     []domain.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}},
		},
		{
			name:     "switching order, two items",
			in:       []config.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.youtube.com/live"}},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}},
			want:     []domain.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.youtube.com/live"}},
		},
		{
			name:     "switching order, several items",
			in:       []config.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}, {URL: "rtmp://rtmp.facebook.com/live"}},
			want:     []domain.Destination{{URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
		},
		{
			name:     "removing all destinations",
			in:       []config.Destination{},
			existing: []domain.Destination{{URL: "rtmp://rtmp.youtube.com/live"}, {URL: "rtmp://rtmp.twitch.tv/live"}, {URL: "rtmp://rtmp.facebook.com/live"}, {URL: "rtmp://rtmp.tiktok.com/live"}},
			want:     []domain.Destination{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveDestinations(tc.existing, tc.in))
		})
	}
}
