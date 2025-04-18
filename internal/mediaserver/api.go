package mediaserver

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type apiPath struct {
	Name   string   `json:"name"`
	Ready  bool     `json:"ready"`
	Tracks []string `json:"tracks"`
}

func fetchPath(apiURL string, httpClient httpClient) (apiPath, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return apiPath{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	httpResp, err := httpClient.Do(req)
	if err != nil {
		return apiPath{}, fmt.Errorf("do request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return apiPath{}, fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return apiPath{}, fmt.Errorf("read body: %w", err)
	}

	var path apiPath
	if err = json.Unmarshal(respBody, &path); err != nil {
		return apiPath{}, fmt.Errorf("unmarshal: %w", err)
	}

	return path, nil
}
