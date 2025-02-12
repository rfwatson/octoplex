package container

import (
	"errors"
	"io"
	"testing"

	"git.netflux.io/rob/termstream/testhelpers"
	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
)

func TestHandleEvents(t *testing.T) {
	var count int
	eventsC1 := make(chan events.Message)
	eventsC2 := make(chan events.Message)
	eventsCFunc := func() <-chan events.Message {
		if count > 0 {
			return eventsC2
		}

		count++
		return eventsC1
	}
	errC := make(chan error)

	containerID := "b905f51b47242090ae504c184c7bc84d6274511ef763c1847039dcaa00a3ad27"
	dockerClient := testhelpers.MockDockerClient{EventsResponse: eventsCFunc, EventsErr: errC}
	logger := testhelpers.NewNopLogger()
	ch := make(chan events.Message)

	done := make(chan struct{})
	go func() {
		defer close(done)

		handleEvents(t.Context(), containerID, &dockerClient, logger, ch)
	}()

	go func() {
		eventsC1 <- events.Message{Action: "start"}
		eventsC1 <- events.Message{Action: "stop"}
		errC <- errors.New("foo")
		eventsC2 <- events.Message{Action: "continue"}
		errC <- io.EOF
	}()

	assert.Equal(t, events.Action("start"), (<-ch).Action)
	assert.Equal(t, events.Action("stop"), (<-ch).Action)
	assert.Equal(t, events.Action("continue"), (<-ch).Action)

	<-done
}
