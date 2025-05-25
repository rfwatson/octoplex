package event_test

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/event"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBus(t *testing.T) {
	bus := event.NewBus(testhelpers.NewTestLogger(t))

	ch1 := bus.Register()
	ch2 := bus.Register()

	evt := event.DestinationAddedEvent{ID: uuid.New()}

	go func() {
		bus.Send(evt)
		bus.Send(evt)
	}()

	assert.Equal(t, evt, (<-ch1).(event.DestinationAddedEvent))
	assert.Equal(t, evt, (<-ch1).(event.DestinationAddedEvent))

	assert.Equal(t, evt, (<-ch2).(event.DestinationAddedEvent))
	assert.Equal(t, evt, (<-ch2).(event.DestinationAddedEvent))

	bus.Deregister(ch1)

	_, ok := <-ch1
	assert.False(t, ok)

	select {
	case <-ch2:
		require.Fail(t, "ch2 should be blocking")
	default:
	}
}
