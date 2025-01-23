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

// Actor is responsible for managing the terminal user interface.
type Actor struct {
	app       *tview.Application
	ch        chan action
	commandCh chan Command
	logger    *slog.Logger
	serverBox *tview.TextView
	destBox   *tview.Table
}

const defaultChanSize = 64

type action func()

// StartActorParams contains the parameters for starting a new terminal user
// interface.
type StartActorParams struct {
	ChanSize int
	Logger   *slog.Logger
}

// StartActor starts the terminal user interface actor.
func StartActor(ctx context.Context, params StartActorParams) (*Actor, error) {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
	ch := make(chan action, chanSize)
	commandCh := make(chan Command, chanSize)

	app := tview.NewApplication()
	serverBox := tview.NewTextView()
	serverBox.SetDynamicColors(true)
	serverBox.SetBorder(true)
	serverBox.SetTitle("media server")
	serverBox.SetTextAlign(tview.AlignCenter)

	destBox := tview.NewTable()
	destBox.SetTitle("destinations")
	destBox.SetBorder(true)
	destBox.SetSelectable(true, false)
	destBox.SetWrapSelection(true, false)
	destBox.SetDoneFunc(func(key tcell.Key) {
		row, _ := destBox.GetSelection()
		commandCh <- CommandToggleDestination{URL: destBox.GetCell(row, 0).Text}
	})

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
	app.SetFocus(destBox)
	app.EnableMouse(false)

	actor := &Actor{
		ch:        ch,
		commandCh: commandCh,
		logger:    params.Logger,
		app:       app,
		serverBox: serverBox,
		destBox:   destBox,
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			app.Stop()
			return nil
		}

		return event
	})
	go actor.actorLoop(ctx)

	return actor, nil
}

// C returns a channel that receives commands from the user interface.
func (a *Actor) C() <-chan Command {
	return a.commandCh
}

func (a *Actor) actorLoop(ctx context.Context) {
	uiDone := make(chan struct{})
	go func() {
		defer func() {
			uiDone <- struct{}{}
		}()

		if err := a.app.Run(); err != nil {
			a.logger.Error("tui application error", "err", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("Context done")
		case <-uiDone:
			close(a.commandCh)
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

	a.destBox.Clear()

	a.destBox.SetCell(0, 0, tview.NewTableCell("[grey]URL").SetAlign(tview.AlignLeft).SetExpansion(7).SetSelectable(false))
	a.destBox.SetCell(0, 1, tview.NewTableCell("[grey]Status").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
	a.destBox.SetCell(0, 2, tview.NewTableCell("[grey]Actions").SetAlign(tview.AlignLeft).SetExpansion(2).SetSelectable(false))

	for i, dest := range state.Destinations {
		a.destBox.SetCell(i+1, 0, tview.NewTableCell(dest.URL))
		a.destBox.SetCell(i+1, 1, tview.NewTableCell("[yellow]off-air"))
		a.destBox.SetCell(i+1, 2, tview.NewTableCell("[green]Tab to go live"))
	}

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
