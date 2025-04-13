package replicator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainerStartErrFromLogs(t *testing.T) {
	testCases := []struct {
		name string
		logs [][]byte
		want error
	}{
		{
			name: "no logs",
			want: errors.New("container failed to start"),
		},
		{
			name: "with only error logs",
			logs: [][]byte{
				[]byte("[error] this is an error log"),
				[]byte("[error] this is another error log"),
			},
			want: errors.New("[error] this is an error log"),
		},
		{
			name: "with only fatal logs",
			logs: [][]byte{
				[]byte("[fatal] this is a fatal log"),
				[]byte("[fatal] this is another fatal log"),
			},
			want: errors.New("[fatal] this is a fatal log"),
		},
		{
			name: "with error and fatal logs",
			logs: [][]byte{
				[]byte("[error] this is an error log"),
				[]byte("[fatal] this is a fatal log"),
			},
			want: errors.New("[fatal] this is a fatal log"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, containerStartErrFromLogs(tc.logs))
		})
	}
}
