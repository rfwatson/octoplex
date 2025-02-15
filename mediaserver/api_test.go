package mediaserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	mocks "git.netflux.io/rob/termstream/generated/mocks/mediaserver"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFetchIngressState(t *testing.T) {
	const URL = "http://localhost:8989/v3/rtmpconns/list"

	testCases := []struct {
		name         string
		httpResponse *http.Response
		httpError    error
		wantState    ingressStreamState
		wantErr      error
	}{
		{
			name:         "non-200 status",
			httpResponse: &http.Response{StatusCode: http.StatusNotFound},
			wantErr:      errors.New("unexpected status code: 404"),
		},
		{
			name: "unparseable response",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("invalid json"))),
			},
			wantErr: errors.New("unmarshal: invalid character 'i' looking for beginning of value"),
		},
		{
			name: "successful response, no streams",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"itemCount":0,"pageCount":0,"items":[]}`))),
			},
			wantState: ingressStreamState{ready: false, listeners: 0},
		},
		{
			name: "successful response, not yet ready",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"itemCount":1,"pageCount":1,"items":[{"id":"d2953cf8-9cd6-4c30-816f-807b80b6a71f","created":"2025-02-15T08:19:00.616220354Z","remoteAddr":"172.17.0.1:32972","state":"publish","path":"live","query":"","bytesReceived":15462,"bytesSent":3467}]}`))),
			},
			wantState: ingressStreamState{ready: false, listeners: 0},
		},
		{
			name: "successful response, ready",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"itemCount":1,"pageCount":1,"items":[{"id":"d2953cf8-9cd6-4c30-816f-807b80b6a71f","created":"2025-02-15T08:19:00.616220354Z","remoteAddr":"172.17.0.1:32972","state":"publish","path":"live","query":"","bytesReceived":27832,"bytesSent":3467}]}`))),
			},
			wantState: ingressStreamState{ready: true, listeners: 0},
		},
		{
			name: "successful response, ready, with listeners",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"itemCount":2,"pageCount":1,"items":[{"id":"12668315-0572-41f1-8384-fe7047cc73be","created":"2025-02-15T08:23:43.836589664Z","remoteAddr":"172.17.0.1:40026","state":"publish","path":"live","query":"","bytesReceived":7180753,"bytesSent":3467},{"id":"079370fd-43bb-4798-b079-860cc3159e4e","created":"2025-02-15T08:24:32.396794364Z","remoteAddr":"192.168.48.3:44736","state":"read","path":"live","query":"","bytesReceived":333435,"bytesSent":24243}]}`))),
			},
			wantState: ingressStreamState{ready: true, listeners: 1},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var httpClient mocks.HTTPClient
			httpClient.
				EXPECT().
				Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == URL && req.Method == http.MethodGet
				})).
				Return(tc.httpResponse, tc.httpError)

			state, err := fetchIngressState(URL, &httpClient)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantState, state)
			}
		})
	}
}
