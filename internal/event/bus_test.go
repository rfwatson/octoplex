package event_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/stretchr/testify/assert"
)

func TestBus(t *testing.T) {
	bus := event.NewBus(testhelpers.NewTestLogger(t))

	ch1 := bus.Register()
	ch2 := bus.Register()

	evt := event.MediaServerStartedEvent{
		RTMPURL:  "rtmp://rtmp.example.com/live",
		RTMPSURL: "rtmps://rtmp.example.com/live",
	}

	go func() {
		bus.Send(evt)
		bus.Send(evt)
	}()

	assert.Equal(t, evt, (<-ch1).(event.MediaServerStartedEvent))
	assert.Equal(t, evt, (<-ch2).(event.MediaServerStartedEvent))
}
