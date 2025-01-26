package mediaserver

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	typescontainer "github.com/docker/docker/api/types/container"

	"git.netflux.io/rob/termstream/container"
	"git.netflux.io/rob/termstream/domain"
)

const (
	imageNameMediaMTX = "netfluxio/mediamtx-alpine:latest"
	rtmpPath          = "live"
)

// action is an action to be performed by the actor.
type action func()

// Actor is responsible for managing the media server.
type Actor struct {
	ch              chan action
	state           *domain.Source
	stateChan       chan domain.Source
	containerClient *container.Actor
	logger          *slog.Logger
	httpClient      *http.Client
}

// StartActorParams contains the parameters for starting a new media server
// actor.
type StartActorParams struct {
	ContainerClient *container.Actor
	ChanSize        int
	Logger          *slog.Logger
}

const (
	defaultChanSize   = 64
	componentName     = "mediaserver"
	httpClientTimeout = time.Second
)

// StartActor starts a new media server actor.
func StartActor(ctx context.Context, params StartActorParams) (*Actor, error) {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)

	actor := &Actor{
		ch:              make(chan action, chanSize),
		state:           new(domain.Source),
		stateChan:       make(chan domain.Source, chanSize),
		containerClient: params.ContainerClient,
		logger:          params.Logger,
		httpClient:      &http.Client{Timeout: httpClientTimeout},
	}

	containerID, containerDone, err := params.ContainerClient.RunContainer(
		ctx,
		container.RunContainerParams{
			Name: "server",
			ContainerConfig: &typescontainer.Config{
				Image: imageNameMediaMTX,
				Env: []string{
					"MTX_LOGLEVEL=debug",
					"MTX_API=yes",
				},
				Labels: map[string]string{
					"component": componentName,
				},
				Healthcheck: &typescontainer.HealthConfig{
					Test:          []string{"CMD", "curl", "-f", "http://localhost:9997/v3/paths/list"},
					Interval:      time.Second * 10,
					StartPeriod:   time.Second * 2,
					StartInterval: time.Second * 2,
					Retries:       2,
				},
			},
			HostConfig: &typescontainer.HostConfig{
				NetworkMode: "host",
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("run container: %w", err)
	}

	actor.state.ContainerID = containerID
	actor.state.URL = "rtmp://localhost:1935/" + rtmpPath

	go actor.actorLoop(containerDone)

	return actor, nil
}

// C returns a channel that will receive the current state of the media server.
func (s *Actor) C() <-chan domain.Source {
	return s.stateChan
}

// State returns the current state of the media server.
func (s *Actor) State() domain.Source {
	resultChan := make(chan domain.Source)
	s.ch <- func() {
		resultChan <- *s.state
	}
	return <-resultChan
}

// Close closes the media server actor.
func (s *Actor) Close() error {
	if err := s.containerClient.RemoveContainers(context.Background(), map[string]string{"component": componentName}); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	return nil
}

func (s *Actor) actorLoop(containerDone <-chan struct{}) {
	defer close(s.ch)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var closing bool
	sendState := func() {
		if !closing {
			s.stateChan <- *s.state
		}
	}

	for {
		select {
		case <-containerDone:
			ticker.Stop()

			s.state.Live = false
			sendState()

			closing = true
			close(s.stateChan)
		case <-ticker.C:
			ingressLive, err := s.fetchIngressStateFromServer()
			if err != nil {
				s.logger.Error("Error fetching server state", "error", err)
				continue
			}
			if ingressLive != s.state.Live {
				s.state.Live = ingressLive
				sendState()
			}
		case action, ok := <-s.ch:
			if !ok {
				return
			}
			action()
		}
	}
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

func (s *Actor) fetchIngressStateFromServer() (bool, error) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost:9997/v3/rtmpconns/list", nil)
	if err != nil {
		return false, fmt.Errorf("new request: %w", err)
	}

	httpResp, err := s.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("do request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", httpResp.StatusCode)
	}

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return false, fmt.Errorf("read body: %w", err)
	}

	var resp apiResponse[rtmpConnsResponse]
	if err = json.Unmarshal(respBody, &resp); err != nil {
		return false, fmt.Errorf("unmarshal: %w", err)
	}

	for _, conn := range resp.Items {
		if conn.Path == rtmpPath && conn.State == "publish" {
			return true, nil
		}
	}

	return false, nil
}
