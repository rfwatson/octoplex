package container

import (
	"io"
	"strings"
	"testing"

	"git.netflux.io/rob/octoplex/internal/container/mocks"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	typescontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetLogs(t *testing.T) {
	var dockerClient mocks.DockerClient
	dockerClient.
		EXPECT().
		ContainerLogs(mock.Anything, "123", typescontainer.LogsOptions{ShowStderr: true, Follow: true}).
		Return(io.NopCloser(strings.NewReader("********line 1\n********line 2\n********line 3\n")), nil)

	ch := make(chan []byte)

	go func() {
		defer close(ch)

		getLogs(
			t.Context(),
			"123",
			&dockerClient,
			LogConfig{Stderr: true},
			ch,
			testhelpers.NewTestLogger(t),
		)
	}()

	// Ensure we get the expected lines, including stripping 8 bytes of Docker
	// multiplexing prefix.
	assert.Equal(t, "line 1", string(<-ch))
	assert.Equal(t, "line 2", string(<-ch))
	assert.Equal(t, "line 3", string(<-ch))

	_, ok := <-ch
	assert.False(t, ok)
}
