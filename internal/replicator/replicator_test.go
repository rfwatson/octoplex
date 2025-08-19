package replicator

import (
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
			want: ErrUnknown,
		},
		{
			name: "unknown host",
			logs: [][]byte{
				[]byte("[tcp @ 0x7f9b86637ec0] [error] Failed to resolve hostname www.example123dkkdkdkd.com: Name does not resolve"),
				[]byte("[rtmp @ 0x7f9b85b88880] [error] Cannot open connection tcp://www.example123dkkdkdkd.com:1935?tcp_nodelay=0"),
				[]byte("[out#0/flv @ 0x7f9b8a5e3780] [fatal] Error opening output rtmp://www.example123dkkdkdkd.com:1935/live: I/O error"),
				[]byte("[error] Error opening output file rtmp://www.nope1840202.com:1935/live."),
				[]byte("[fatal] Error opening output files: I/O error"),
			},
			want: ErrUnknownHost,
		},
		{
			name: "connection failed",
			logs: [][]byte{
				[]byte("[aost#0:1/copy @ 0x7fadb1ff2340] [error] Error submitting a packet to the muxer: Broken pipe"),
				[]byte("[aost#0:1/copy @ 0x7fadb1ff2340] [error] Error submitting a packet to the muxer: Broken pipe"),
				[]byte("[out#0/flv @ 0x7fadb5b5f780] [error] Error muxing a packet"),
				[]byte("[out#0/flv @ 0x7fadb5b5f780] [error] Task finished with error code: -32 (Broken pipe)"),
				[]byte("[out#0/flv @ 0x7fadb5b5f780] [error] Terminating thread with return code -32 (Broken pipe)"),
				[]byte("[out#0/flv @ 0x7fadb5b5f780] [error] Error writing trailer: Broken pipe"),
				[]byte("[out#0/flv @ 0x7fadb5b5f780] [error] Error closing file: Broken pipe"),
			},
			want: ErrConnectionFailed,
		},
		{
			name: "timeout",
			logs: [][]byte{
				[]byte("[tcp @ 0x7fd32f28e640] [error] Connection to tcp://www.example.com:1935?tcp_nodelay=0 failed: Operation timed out"),
				[]byte("[rtmp @ 0x7fd32f28e400] [error] Cannot open connection tcp://www.example.com:1935?tcp_nodelay=0"),
				[]byte("[out#0/flv @ 0x7fd333cdb780] [fatal] Error opening output rtmp://www.example.com:1935/live: Operation timed out"),
				[]byte("[error] Error opening output file rtmp://www.example.com:1935/live."),
				[]byte("[fatal] Error opening output files: Operation timed out"),
			},
			want: ErrTimeout,
		},
		{
			name: "authentication failed",
			logs: [][]byte{
				[]byte("[error] Authentication failed: Access denied"), // contrived
			},
			want: ErrForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, startErrFromLogs(tc.logs))
		})
	}
}
