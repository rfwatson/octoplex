package container

import (
	"errors"
	"io"
	"testing"

	containermocks "git.netflux.io/rob/octoplex/internal/generated/mocks/container"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandleEvents(t *testing.T) {
	eventsC1 := make(chan events.Message)
	eventsC2 := make(chan events.Message)
	errC := make(chan error)

	containerID := "b905f51b47242090ae504c184c7bc84d6274511ef763c1847039dcaa00a3ad27"

	var dockerClient containermocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		Events(mock.Anything, mock.Anything).
		Return(eventsC1, errC).
		Once()
	dockerClient.
		EXPECT().
		Events(mock.Anything, mock.Anything).
		Return(eventsC2, errC).
		Once()

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
