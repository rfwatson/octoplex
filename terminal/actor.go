package terminal

import (
	"cmp"
	"context"
	"log/slog"
	"strings"

	"git.netflux.io/rob/termstream/domain"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const defaultChanSize = 64

type action func()

// Actor is responsible for managing the terminal user interface.
type Actor struct {
	app       *tview.Application
	ch        chan action
	doneCh    chan struct{}
	logger    *slog.Logger
	serverBox *tview.TextView
}

// StartActorParams contains the parameters for starting a new terminal user
// interface.
type StartActorParams struct {
	ChanSize int
	Logger   *slog.Logger
}

// StartActor starts the terminal user interface actor.
func StartActor(ctx context.Context, params StartActorParams) (*Actor, error) {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)

	app := tview.NewApplication()
	serverBox := tview.NewTextView()
	serverBox.SetDynamicColors(true)
	serverBox.SetBorder(true)
	serverBox.SetTitle("media server")
	serverBox.SetTextAlign(tview.AlignCenter)

	destBox := tview.NewBox().
		SetBorder(true).
		SetTitle("destinations")

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(serverBox, 9, 0, false).
		AddItem(destBox, 0, 1, false)

	container := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(flex, 120, 0, false).
		AddItem(nil, 0, 1, false)

	app.SetRoot(container, true)
	app.EnableMouse(true)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			app.Stop()
			return nil
		}

		return event
	})

	actor := &Actor{
		ch:        make(chan action, chanSize),
		doneCh:    make(chan struct{}, 1),
		logger:    params.Logger,
		app:       app,
		serverBox: serverBox,
	}

	go actor.actorLoop(ctx)

	return actor, nil
}

// C returns a channel that is closed when the terminal user interface closes.
func (a *Actor) C() <-chan struct{} {
	return a.doneCh
}

func (a *Actor) actorLoop(ctx context.Context) {
	uiDone := make(chan struct{})
	go func() {
		defer close(uiDone)

		if err := a.app.Run(); err != nil {
			a.logger.Error("tui application error", "err", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("Context done")
		case <-uiDone:
			a.doneCh <- struct{}{}
		case action, ok := <-a.ch:
			if !ok {
				return
			}
			action()
		}
	}
}

// SetState sets the state of the terminal user interface.
func (a *Actor) SetState(state domain.AppState) {
	a.ch <- func() {
		a.redrawFromState(state)
	}
}

func (a *Actor) redrawFromState(state domain.AppState) {
	a.serverBox.SetText(generateServerStatus(state))
	a.app.Draw()
}

func generateServerStatus(state domain.AppState) string {
	var s strings.Builder

	s.WriteString("\n")

	s.WriteString("[grey]Container status: ")
	if state.ContainerRunning {
		s.WriteString("[green]running")
	} else {
		s.WriteString("[red]stopped")
	}
	s.WriteString("\n\n")

	s.WriteString("[grey]RTMP URL: ")
	if state.IngressURL != "" {
		s.WriteString("[white:grey]" + state.IngressURL)
	}
	s.WriteString("\n\n")

	s.WriteString("[grey:black]Ingress stream: ")
	if state.IngressLive {
		s.WriteString("[green]on-air")
	} else {
		s.WriteString("[yellow]off-air")
	}

	return s.String()
}

// Close closes the terminal user interface.
func (a *Actor) Close() {
	a.app.Stop()
}
