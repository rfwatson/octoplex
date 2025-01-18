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

	"git.netflux.io/rob/termstream/container"
)

const imageNameMediaMTX = "bluenviron/mediamtx"

// State contains the current state of the media server.
type State struct {
	Live bool
}

// action is an action to be performed by the actor.
type action func()

// Actor is responsible for managing the media server.
type Actor struct {
	ch         chan action
	stateChan  chan State
	runner     *container.Runner
	logger     *slog.Logger
	httpClient *http.Client

	// mutable state
	live bool
}

// StartActorParams contains the parameters for starting a new media server
// actor.
type StartActorParams struct {
	Runner   *container.Runner
	ChanSize int
	Logger   *slog.Logger
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
		ch:         make(chan action, chanSize),
		stateChan:  make(chan State, chanSize),
		runner:     params.Runner,
		logger:     params.Logger,
		httpClient: &http.Client{Timeout: httpClientTimeout},
	}

	containerDone, err := params.Runner.RunContainer(
		ctx,
		container.RunContainerParams{
			Name:  "server",
			Image: imageNameMediaMTX,
			Env: []string{
				"MTX_LOG_LEVEL=debug",
				"MTX_API=yes",
			},
			Labels: map[string]string{
				"component": componentName,
			},
			NetworkMode: "host",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("run container: %w", err)
	}

	go actor.actorLoop(containerDone)

	return actor, nil
}

// C returns a channel that will receive the current state of the media server.
func (s *Actor) C() <-chan State {
	return s.stateChan
}

func (s *Actor) State() State {
	resultChan := make(chan State)

	s.ch <- func() {
		resultChan <- State{Live: s.live}
	}

	return <-resultChan
}

// Close closes the media server actor.
func (s *Actor) Close() error {
	if err := s.runner.RemoveContainers(context.Background(), map[string]string{"component": componentName}); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}

	return nil
}

func (s *Actor) actorLoop(containerDone <-chan struct{}) {
	defer close(s.ch)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-containerDone:
			s.stateChan <- State{Live: false}
			close(s.stateChan)
		case <-ticker.C:
			live, err := s.checkState()
			if err != nil {
				s.logger.Error("Error fetching server state", "error", err)
				continue
			}
			if live != s.live {
				s.live = live
				s.stateChan <- State{Live: live}
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

func (s *Actor) checkState() (bool, error) {
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
		if conn.Path == "live" && conn.State == "publish" {
			return true, nil
		}
	}

	return false, nil
}
