package terminal

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"

	"git.netflux.io/rob/termstream/domain"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Actor is responsible for managing the terminal user interface.
type Actor struct {
	app        *tview.Application
	ch         chan action
	commandCh  chan Command
	logger     *slog.Logger
	sourceView *tview.Table
	destView   *tview.Table
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

	sourceView := tview.NewTable()
	sourceView.SetTitle("source")
	sourceView.SetBorder(true)

	destView := tview.NewTable()
	destView.SetTitle("destinations")
	destView.SetBorder(true)
	destView.SetSelectable(true, false)
	destView.SetWrapSelection(true, false)
	destView.SetDoneFunc(func(key tcell.Key) {
		row, _ := destView.GetSelection()
		commandCh <- CommandToggleDestination{URL: destView.GetCell(row, 0).Text}
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(sourceView, 4, 0, false).
		AddItem(destView, 0, 1, false)

	container := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(flex, 180, 0, false).
		AddItem(nil, 0, 1, false)

	app.SetRoot(container, true)
	app.SetFocus(destView)
	app.EnableMouse(false)

	actor := &Actor{
		ch:         ch,
		commandCh:  commandCh,
		logger:     params.Logger,
		app:        app,
		sourceView: sourceView,
		destView:   destView,
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
	setHeaderRow := func(tableView *tview.Table) {
		tableView.SetCell(0, 0, tview.NewTableCell("[grey]URL").SetAlign(tview.AlignLeft).SetExpansion(7).SetSelectable(false))
		tableView.SetCell(0, 1, tview.NewTableCell("[grey]Stream").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
		tableView.SetCell(0, 2, tview.NewTableCell("[grey]Container").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
		tableView.SetCell(0, 3, tview.NewTableCell("[grey]Health").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
		tableView.SetCell(0, 4, tview.NewTableCell("[grey]CPU %").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
		tableView.SetCell(0, 5, tview.NewTableCell("[grey]Mem used (MB)").SetAlign(tview.AlignLeft).SetExpansion(1).SetSelectable(false))
		tableView.SetCell(0, 6, tview.NewTableCell("[grey]Actions").SetAlign(tview.AlignLeft).SetExpansion(2).SetSelectable(false))
	}

	a.sourceView.Clear()
	setHeaderRow(a.sourceView)
	a.sourceView.SetCell(1, 0, tview.NewTableCell(state.Source.URL))

	if state.Source.Live {
		a.sourceView.SetCell(1, 1, tview.NewTableCell("[black:green]receiving"))
	} else {
		a.sourceView.SetCell(1, 1, tview.NewTableCell("[yellow]off-air"))
	}
	a.sourceView.SetCell(1, 2, tview.NewTableCell("[white]"+state.Source.Container.State))
	a.sourceView.SetCell(1, 3, tview.NewTableCell("[white]"+cmp.Or(state.Source.Container.HealthState, "starting")))
	a.sourceView.SetCell(1, 4, tview.NewTableCell("[white]"+fmt.Sprintf("%.1f", state.Source.Container.CPUPercent)))
	a.sourceView.SetCell(1, 5, tview.NewTableCell("[white]"+fmt.Sprintf("%.1f", float64(state.Source.Container.MemoryUsageBytes)/1024/1024)))
	a.sourceView.SetCell(1, 6, tview.NewTableCell(""))

	a.destView.Clear()
	setHeaderRow(a.destView)

	for i, dest := range state.Destinations {
		a.destView.SetCell(i+1, 0, tview.NewTableCell(dest.URL))
		if dest.Live {
			a.destView.SetCell(i+1, 1, tview.NewTableCell("[black:green]sending"))
		} else {
			a.destView.SetCell(i+1, 1, tview.NewTableCell("[white]off-air"))
		}
		a.destView.SetCell(i+1, 2, tview.NewTableCell("[white]"+cmp.Or(dest.Container.State, "-")))

		healthState := "-"
		if dest.Container.State == "running" {
			healthState = "healthy"
		}
		a.destView.SetCell(i+1, 3, tview.NewTableCell("[white]"+healthState))

		cpuPercent := "-"
		if dest.Container.State == "running" {
			cpuPercent = fmt.Sprintf("%.1f", dest.Container.CPUPercent)
		}

		memoryUsage := "-"
		if dest.Container.State == "running" {
			memoryUsage = fmt.Sprintf("%.1f", float64(dest.Container.MemoryUsageBytes)/1024/1024)
		}

		a.destView.SetCell(i+1, 4, tview.NewTableCell("[white]"+cpuPercent))
		a.destView.SetCell(i+1, 5, tview.NewTableCell("[white]"+memoryUsage))
		a.destView.SetCell(i+1, 6, tview.NewTableCell("[green]Tab to go live"))
	}

	a.app.Draw()
}

// Close closes the terminal user interface.
func (a *Actor) Close() {
	a.app.Stop()
}
