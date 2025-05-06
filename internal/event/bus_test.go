package event_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, evt, (<-ch1).(event.MediaServerStartedEvent))

	assert.Equal(t, evt, (<-ch2).(event.MediaServerStartedEvent))
	assert.Equal(t, evt, (<-ch2).(event.MediaServerStartedEvent))

	bus.Deregister(ch1)

	_, ok := <-ch1
	assert.False(t, ok)

	select {
	case <-ch2:
		require.Fail(t, "ch2 should be blocking")
	default:
	}
}
