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
	pages      *tview.Pages
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
	destView.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGrey))
	destView.SetDoneFunc(func(key tcell.Key) {
		const urlCol = 1
		row, _ := destView.GetSelection()
		url, ok := destView.GetCell(row, urlCol).GetReference().(string)
		if !ok {
			return
		}

		commandCh <- CommandToggleDestination{URL: url}
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

	pages := tview.NewPages()
	pages.AddPage("main", container, true, true)

	app.SetRoot(pages, true)
	app.SetFocus(destView)
	app.EnableMouse(false)

	actor := &Actor{
		ch:         ch,
		commandCh:  commandCh,
		logger:     params.Logger,
		app:        app,
		pages:      pages,
		sourceView: sourceView,
		destView:   destView,
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			modal := tview.NewModal()
			modal.SetText("Are you sure you want to quit?").
				AddButtons([]string{"Quit", "Cancel"}).
				SetBackgroundColor(tcell.ColorBlack).
				SetTextColor(tcell.ColorWhite).
				SetDoneFunc(func(buttonIndex int, _ string) {
					if buttonIndex == 1 || buttonIndex == -1 {
						pages.RemovePage("modal")
						app.SetFocus(destView)
						return
					}

					commandCh <- CommandQuit{}
				})
			modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

			pages.AddPage("modal", modal, true, true)
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
		if state.Source.ExitReason != "" {
			modal := tview.NewModal()
			modal.SetText("Mediaserver error: " + state.Source.ExitReason).
				AddButtons([]string{"Quit"}).
				SetBackgroundColor(tcell.ColorBlack).
				SetTextColor(tcell.ColorWhite).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					// TODO: improve app cleanup
					a.app.Stop()
				})
			modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

			a.pages.AddPage("modal", modal, true, true)
		}

		a.redrawFromState(state)
	}
}

const dash = "—"

func (a *Actor) redrawFromState(state domain.AppState) {
	headerCell := func(content string, expansion int) *tview.TableCell {
		return tview.
			NewTableCell(content).
			SetExpansion(expansion).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
	}

	setHeaderRow := func(tableView *tview.Table, txRxLabel string) {
		tableView.SetCell(0, 0, headerCell("[grey]Name", 3))
		tableView.SetCell(0, 1, headerCell("[grey]URL", 3))
		tableView.SetCell(0, 2, headerCell("[grey]Status", 2))
		tableView.SetCell(0, 3, headerCell("[grey]Container", 2))
		tableView.SetCell(0, 4, headerCell("[grey]Health", 2))
		tableView.SetCell(0, 5, headerCell("[grey]CPU %", 1))
		tableView.SetCell(0, 6, headerCell("[grey]Memory MB", 1))
		tableView.SetCell(0, 7, headerCell("[grey]"+txRxLabel+" Kbps", 1))
		tableView.SetCell(0, 8, headerCell("[grey]Action", 2))
	}

	a.sourceView.Clear()
	setHeaderRow(a.sourceView, "Rx")
	a.sourceView.SetCell(1, 0, tview.NewTableCell("Local server"))
	a.sourceView.SetCell(1, 1, tview.NewTableCell(state.Source.RTMPURL))

	if state.Source.Live {
		a.sourceView.SetCell(1, 2, tview.NewTableCell("[black:green]receiving"))
	} else if state.Source.Container.State == "running" && state.Source.Container.HealthState == "healthy" {
		a.sourceView.SetCell(1, 2, tview.NewTableCell("[black:yellow]ready"))
	} else {
		a.sourceView.SetCell(1, 2, tview.NewTableCell("[white:red]not ready"))
	}
	a.sourceView.SetCell(1, 3, tview.NewTableCell("[white]"+state.Source.Container.State))
	a.sourceView.SetCell(1, 4, tview.NewTableCell("[white]"+cmp.Or(state.Source.Container.HealthState, "starting")))
	a.sourceView.SetCell(1, 5, tview.NewTableCell("[white]"+fmt.Sprintf("%.1f", state.Source.Container.CPUPercent)))
	a.sourceView.SetCell(1, 6, tview.NewTableCell("[white]"+fmt.Sprintf("%.1f", float64(state.Source.Container.MemoryUsageBytes)/1024/1024)))
	a.sourceView.SetCell(1, 7, tview.NewTableCell("[white]"+fmt.Sprintf("%d", state.Source.Container.RxRate)))
	a.sourceView.SetCell(1, 8, tview.NewTableCell(""))

	a.destView.Clear()
	setHeaderRow(a.destView, "Tx")

	for i, dest := range state.Destinations {
		a.destView.SetCell(i+1, 0, tview.NewTableCell(dest.Name))
		a.destView.SetCell(i+1, 1, tview.NewTableCell(truncate(dest.URL, 50)).SetReference(dest.URL))
		switch dest.Status {
		case domain.DestinationStatusLive:
			a.destView.SetCell(
				i+1,
				2,
				tview.NewTableCell("sending").
					SetTextColor(tcell.ColorBlack).
					SetBackgroundColor(tcell.ColorGreen).
					SetSelectedStyle(
						tcell.
							StyleDefault.
							Foreground(tcell.ColorBlack).
							Background(tcell.ColorGreen),
					),
			)
		case domain.DestinationStatusStarting:
			label := "starting"
			if dest.Container.RestartCount > 0 {
				label = fmt.Sprintf("restarting (%d)", dest.Container.RestartCount)
			}
			a.destView.SetCell(i+1, 2, tview.NewTableCell("[white]"+label))
		case domain.DestinationStatusOffAir:
			a.destView.SetCell(i+1, 2, tview.NewTableCell("[white]off-air"))
		default:
			panic("unknown destination state")
		}
		a.destView.SetCell(i+1, 3, tview.NewTableCell("[white]"+cmp.Or(dest.Container.State, dash)))

		healthState := dash
		if dest.Status == domain.DestinationStatusLive {
			healthState = "healthy"
		}
		a.destView.SetCell(i+1, 4, tview.NewTableCell("[white]"+healthState))

		cpuPercent := dash
		if dest.Container.State == "running" {
			cpuPercent = fmt.Sprintf("%.1f", dest.Container.CPUPercent)
		}
		a.destView.SetCell(i+1, 5, tview.NewTableCell("[white]"+cpuPercent))

		memoryUsage := dash
		if dest.Container.State == "running" {
			memoryUsage = fmt.Sprintf("%.1f", float64(dest.Container.MemoryUsageBytes)/1000/1000)
		}
		a.destView.SetCell(i+1, 6, tview.NewTableCell("[white]"+memoryUsage))

		txRate := dash
		if dest.Container.State == "running" {
			txRate = "[white]" + fmt.Sprintf("%d", dest.Container.TxRate)
		}
		a.destView.SetCell(i+1, 7, tview.NewTableCell(txRate))

		a.destView.SetCell(i+1, 8, tview.NewTableCell("[green]Tab to go live"))
	}

	a.app.Draw()
}

// Close closes the terminal user interface.
func (a *Actor) Close() {
	a.app.Stop()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
