package container

import (
	"bytes"
	_ "embed"
	"io"
	"testing"

	"git.netflux.io/rob/octoplex/internal/container/mocks"
	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/testhelpers"
	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/pull_progress.json
var pullProgressJSON []byte

func TestHandleImagePull(t *testing.T) {
	logger := testhelpers.NewTestLogger(t)

	const imageName = "alpine"
	containerStateC := make(chan domain.Container)

	var dockerClient mocks.DockerClient
	defer dockerClient.AssertExpectations(t)

	dockerClient.
		EXPECT().
		ImagePull(mock.Anything, imageName, image.PullOptions{}).
		Return(io.NopCloser(bytes.NewReader(pullProgressJSON)), nil)

	var containerStates []domain.Container

	go func() {
		require.NoError(t, handleImagePull(t.Context(), imageName, &dockerClient, containerStateC, logger))
	}()

	const expectedContainerStates = 46
	for range expectedContainerStates {
		containerStates = append(containerStates, <-containerStateC)
	}

	assert.Len(t, containerStates, expectedContainerStates)

	for _, containerState := range containerStates {
		assert.Equal(t, domain.ContainerStatusPulling, containerState.Status)
		assert.Equal(t, imageName, containerState.ImageName)
		assert.NotZero(t, containerState.PullStatus)
	}
}
