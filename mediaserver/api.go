package mediaserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type apiResponse[T any] struct {
	Items []T `json:"items"`
}

type rtmpConnsResponse struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created"`
	State         string    `json:"state"`
	Path          string    `json:"path"`
	BytesReceived int64     `json:"bytesReceived"`
	BytesSent     int64     `json:"bytesSent"`
	RemoteAddr    string    `json:"remoteAddr"`
}

type ingressStreamState struct {
	ready     bool
	listeners int
}

func fetchIngressState(apiURL string, httpClient httpClient) (state ingressStreamState, _ error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return state, fmt.Errorf("new request: %w", err)
	}

	httpResp, err := httpClient.Do(req)
	if err != nil {
		return state, fmt.Errorf("do request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return state, fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return state, fmt.Errorf("read body: %w", err)
	}

	var resp apiResponse[rtmpConnsResponse]
	if err = json.Unmarshal(respBody, &resp); err != nil {
		return state, fmt.Errorf("unmarshal: %w", err)
	}

	for _, conn := range resp.Items {
		if conn.Path != rtmpPath {
			continue
		}

		switch conn.State {
		case "publish":
			// mediamtx may report a stream as being in publish state via the API,
			// but still refuse to serve them due to being unpublished. This seems to
			// be a bug, this is a hacky workaround.
			state.ready = conn.BytesReceived > 20_000
		case "read":
			state.listeners++
		}
	}

	return state, nil
}
