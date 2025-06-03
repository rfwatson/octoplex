package mediaserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	mediaservermocks "git.netflux.io/rob/octoplex/internal/generated/mocks/mediaserver"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFetchPath(t *testing.T) {
	const url = "http://localhost:8989/v3/paths/get/live"

	testCases := []struct {
		name         string
		httpResponse *http.Response
		httpError    error
		wantPath     apiPath
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
			name: "successful response, not ready",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"name":"live","confName":"live","source":null,"ready":false,"readyTime":null,"tracks":[],"bytesReceived":0,"bytesSent":0,"readers":[]}`))),
			},
			wantPath: apiPath{Name: "live", Ready: false, Tracks: []string{}},
		},
		{
			name: "successful response, ready",
			httpResponse: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"name":"live","confName":"live","source":{"type":"rtmpConn","id":"fd2d79a8-bab9-4141-a1b5-55bd1a8649df"},"ready":true,"readyTime":"2025-04-18T07:44:53.683627506Z","tracks":["H264"],"bytesReceived":254677,"bytesSent":0,"readers":[]}`))),
			},
			wantPath: apiPath{Name: "live", Ready: true, Tracks: []string{"H264"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var httpClient mediaservermocks.HTTPClient
			httpClient.
				EXPECT().
				Do(mock.MatchedBy(func(req *http.Request) bool {
					return req.URL.String() == url && req.Method == http.MethodGet
				})).
				Return(tc.httpResponse, tc.httpError)

			path, err := fetchPath(url, &httpClient)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantPath, path)
			}
		})
	}
}
