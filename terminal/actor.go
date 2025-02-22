package terminal

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"git.netflux.io/rob/termstream/domain"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type sourceViews struct {
	url    *tview.TextView
	status *tview.TextView
	tracks *tview.TextView
	health *tview.TextView
	cpu    *tview.TextView
	mem    *tview.TextView
	rx     *tview.TextView
}

// Actor is responsible for managing the terminal user interface.
type Actor struct {
	app         *tview.Application
	pages       *tview.Pages
	ch          chan action
	commandCh   chan Command
	logger      *slog.Logger
	sourceViews sourceViews
	destView    *tview.Table
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

	sidebar := tview.NewFlex()
	sidebar.SetDirection(tview.FlexRow)

	sourceView := tview.NewFlex()
	sourceView.SetDirection(tview.FlexColumn)
	sourceView.SetBorder(true)
	sourceView.SetTitle("Ingress RTMP server")
	sidebar.AddItem(sourceView, 9, 0, false)

	leftCol := tview.NewFlex()
	leftCol.SetDirection(tview.FlexRow)
	rightCol := tview.NewFlex()
	rightCol.SetDirection(tview.FlexRow)
	sourceView.AddItem(leftCol, 9, 0, false)
	sourceView.AddItem(rightCol, 0, 1, false)

	urlHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerURL)
	leftCol.AddItem(urlHeaderTextView, 1, 0, false)
	urlTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(urlTextView, 1, 0, false)

	statusHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerStatus)
	leftCol.AddItem(statusHeaderTextView, 1, 0, false)
	statusTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(statusTextView, 1, 0, false)

	tracksHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerTracks)
	leftCol.AddItem(tracksHeaderTextView, 1, 0, false)
	tracksTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(tracksTextView, 1, 0, false)

	healthHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerHealth)
	leftCol.AddItem(healthHeaderTextView, 1, 0, false)
	healthTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(healthTextView, 1, 0, false)

	cpuHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerCPU)
	leftCol.AddItem(cpuHeaderTextView, 1, 0, false)
	cpuTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(cpuTextView, 1, 0, false)

	memHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerMem)
	leftCol.AddItem(memHeaderTextView, 1, 0, false)
	memTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(memTextView, 1, 0, false)

	rxHeaderTextView := tview.NewTextView().SetDynamicColors(true).SetText("[grey]" + headerRx)
	leftCol.AddItem(rxHeaderTextView, 1, 0, false)
	rxTextView := tview.NewTextView().SetDynamicColors(true).SetText("[white]" + dash)
	rightCol.AddItem(rxTextView, 1, 0, false)

	aboutView := tview.NewBox()
	aboutView.SetBorder(true)
	aboutView.SetTitle("Actions")
	sidebar.AddItem(aboutView, 0, 1, false)

	destView := tview.NewTable()
	destView.SetTitle("Egress streams")
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
		SetDirection(tview.FlexColumn).
		AddItem(sidebar, 40, 0, false).
		AddItem(destView, 0, 6, false)

	pages := tview.NewPages()
	pages.AddPage("main", flex, true, true)

	app.SetRoot(pages, true)
	app.SetFocus(destView)
	app.EnableMouse(false)

	actor := &Actor{
		ch:        ch,
		commandCh: commandCh,
		logger:    params.Logger,
		app:       app,
		pages:     pages,
		sourceViews: sourceViews{
			url:    urlTextView,
			status: statusTextView,
			tracks: tracksTextView,
			health: healthTextView,
			cpu:    cpuTextView,
			mem:    memTextView,
			rx:     rxTextView,
		},
		destView: destView,
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

const (
	headerName      = "Name"
	headerURL       = "URL"
	headerStatus    = "Status"
	headerContainer = "Container"
	headerHealth    = "Health"
	headerCPU       = "CPU %"
	headerMem       = "Mem M"
	headerRx        = "Rx Kbps"
	headerTx        = "Tx Kbps"
	headerAction    = "Action"
	headerTracks    = "Tracks"
)

func (a *Actor) redrawFromState(state domain.AppState) {
	headerCell := func(content string, expansion int) *tview.TableCell {
		return tview.
			NewTableCell(content).
			SetExpansion(expansion).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
	}

	a.sourceViews.url.SetText(state.Source.RTMPURL)

	tracks := dash
	if state.Source.Live && len(state.Source.Tracks) > 0 {
		tracks = strings.Join(state.Source.Tracks, ", ")
	}
	a.sourceViews.tracks.SetText(tracks)

	if state.Source.Live {
		a.sourceViews.status.SetText("[black:green]receiving")
	} else if state.Source.Container.State == "running" && state.Source.Container.HealthState == "healthy" {
		a.sourceViews.status.SetText("[black:yellow]ready")
	} else {
		a.sourceViews.status.SetText("[white:red]not ready")
	}

	a.sourceViews.health.SetText("[white]" + cmp.Or(state.Source.Container.HealthState, dash))

	cpuPercent := dash
	if state.Source.Container.State == "running" {
		cpuPercent = fmt.Sprintf("%.1f", state.Source.Container.CPUPercent)
	}
	a.sourceViews.cpu.SetText("[white]" + cpuPercent)

	memUsage := dash
	if state.Source.Container.State == "running" {
		memUsage = fmt.Sprintf("%.1f", float64(state.Source.Container.MemoryUsageBytes)/1024/1024)
	}
	a.sourceViews.mem.SetText("[white]" + memUsage)

	rxRate := dash
	if state.Source.Container.State == "running" {
		rxRate = fmt.Sprintf("%d", state.Source.Container.RxRate)
	}
	a.sourceViews.rx.SetText("[white]" + rxRate)

	a.destView.Clear()
	a.destView.SetCell(0, 0, headerCell("[grey]"+headerName, 3))
	a.destView.SetCell(0, 1, headerCell("[grey]"+headerURL, 3))
	a.destView.SetCell(0, 2, headerCell("[grey]"+headerStatus, 2))
	a.destView.SetCell(0, 3, headerCell("[grey]"+headerContainer, 2))
	a.destView.SetCell(0, 4, headerCell("[grey]"+headerHealth, 2))
	a.destView.SetCell(0, 5, headerCell("[grey]"+headerCPU, 1))
	a.destView.SetCell(0, 6, headerCell("[grey]"+headerMem, 1))
	a.destView.SetCell(0, 7, headerCell("[grey]"+headerTx, 1))

	for i, dest := range state.Destinations {
		a.destView.SetCell(i+1, 0, tview.NewTableCell(dest.Name))
		a.destView.SetCell(i+1, 1, tview.NewTableCell(dest.URL).SetReference(dest.URL).SetMaxWidth(20))
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
