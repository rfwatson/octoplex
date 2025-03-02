package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			name:  "string with equal lenth to required width",
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
