package mediaserver

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

func buildAPIClient(certPEM []byte) (*http.Client, error) {
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certPEM) {
		return nil, errors.New("failed to add certificate to pool")
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					cert, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						return fmt.Errorf("parse certificate: %w", err)
					}

					if _, err := cert.Verify(x509.VerifyOptions{Roots: certPool}); err != nil {
						return fmt.Errorf("TLS verification: %w", err)
					}

					return nil
				},
			},
		},
	}, nil
}

const userAgent = "octoplex-client"

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

// TODO: handle pagination
func fetchIngressState(apiURL string, streamKey StreamKey, httpClient httpClient) (state ingressStreamState, _ error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return state, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

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
		if conn.Path != string(streamKey) {
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

type path struct {
	Name   string   `json:"name"`
	Tracks []string `json:"tracks"`
}

// TODO: handle pagination
func fetchTracks(apiURL string, streamKey StreamKey, httpClient httpClient) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	httpResp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var resp apiResponse[path]
	if err = json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	var tracks []string
	for _, path := range resp.Items {
		if path.Name == string(streamKey) {
			tracks = path.Tracks
		}
	}

	return tracks, nil
}
