package terminal

import (
	"testing"

	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestContainerStateToStartState(t *testing.T) {
	testCases := []struct {
		containerState string
		want           startState
	}{
		{
			containerState: domain.ContainerStatusPulling,
			want:           startStateStarting,
		},
		{
			containerState: domain.ContainerStatusCreated,
			want:           startStateStarting,
		},
		{
			containerState: domain.ContainerStatusRunning,
			want:           startStateStarted,
		},
		{
			containerState: domain.ContainerStatusRestarting,
			want:           startStateStarted,
		},
		{
			containerState: domain.ContainerStatusPaused,
			want:           startStateStarted,
		},
		{
			containerState: domain.ContainerStatusRemoving,
			want:           startStateStarted,
		},
		{
			containerState: domain.ContainerStatusExited,
			want:           startStateNotStarted,
		},
		{
			containerState: domain.ContainerStatusDead,
			want:           startStateNotStarted,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.containerState, func(t *testing.T) {
			assert.Equal(t, tc.want, containerStateToStartState(tc.containerState))
		})
	}
}

func TestRightPad(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "short string",
			input: "foo",
			want:  "foo   ",
		},
		{
			name:  "string with length equal to required width",
			input: "foobar",
			want:  "foobar",
		},
		{
			name:  "long string",
			input: "foobarbaz",
			want:  "foobar",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rightPad(tc.input, 6))
		})
	}
}
