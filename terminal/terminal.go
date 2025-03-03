package terminal

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"git.netflux.io/rob/octoplex/domain"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.design/x/clipboard"
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

// UI is responsible for managing the terminal user interface.
type UI struct {
	app         *tview.Application
	pages       *tview.Pages
	commandCh   chan Command
	buildInfo   domain.BuildInfo
	logger      *slog.Logger
	sourceViews sourceViews
	destView    *tview.Table
}

// StartParams contains the parameters for starting a new terminal user
// interface.
type StartParams struct {
	ChanSize           int
	Logger             *slog.Logger
	ClipboardAvailable bool
	BuildInfo          domain.BuildInfo
}

const defaultChanSize = 64

// StartUI starts the terminal user interface.
func StartUI(ctx context.Context, params StartParams) (*UI, error) {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
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

	aboutView := tview.NewFlex()
	aboutView.SetDirection(tview.FlexRow)
	aboutView.SetBorder(true)
	aboutView.SetTitle("Actions")
	aboutView.AddItem(tview.NewTextView().SetText("[C] Copy ingress RTMP URL"), 1, 0, false)
	aboutView.AddItem(tview.NewTextView().SetText("[?] About"), 1, 0, false)

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

	ui := &UI{
		commandCh: commandCh,
		buildInfo: params.BuildInfo,
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
		case tcell.KeyRune:
			switch event.Rune() {
			case 'c', 'C':
				ui.copySourceURLToClipboard(params.ClipboardAvailable)
			case '?':
				ui.showAbout()
			}
		case tcell.KeyCtrlC:
			ui.confirmQuit()
			return nil
		}

		return event
	})

	go ui.run(ctx)

	return ui, nil
}

// C returns a channel that receives commands from the user interface.
func (ui *UI) C() <-chan Command {
	return ui.commandCh
}

func (ui *UI) run(ctx context.Context) {
	defer close(ui.commandCh)

	uiDone := make(chan struct{})
	go func() {
		defer func() {
			uiDone <- struct{}{}
		}()

		if err := ui.app.Run(); err != nil {
			ui.logger.Error("tui application error", "err", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-uiDone:
			return
		}
	}
}

// ShowStartupCheckModal shows a modal dialog to the user, asking if they want
// to kill a running instance of Octoplex.
//
// The method will block until the user has made a choice, after which the
// channel will receive true if the user wants to quit the other instance, or
// false to quit this instance.
func (ui *UI) ShowStartupCheckModal() bool {
	done := make(chan bool)

	ui.app.QueueUpdateDraw(func() {
		modal := tview.NewModal()
		modal.SetText("Another instance of Octoplex may already be running. Pressing continue will close that instance. Continue?").
			AddButtons([]string{"Continue", "Exit"}).
			SetBackgroundColor(tcell.ColorBlack).
			SetTextColor(tcell.ColorWhite).
			SetDoneFunc(func(buttonIndex int, _ string) {
				if buttonIndex == 0 {
					ui.pages.RemovePage("modal")
					ui.app.SetFocus(ui.destView)
					done <- true
				} else {
					done <- false
				}
			})
		modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

		ui.pages.AddPage("modal", modal, true, true)
	})

	return <-done
}

func (ui *UI) handleMediaServerClosed(exitReason string) {
	done := make(chan struct{})

	ui.app.QueueUpdateDraw(func() {
		modal := tview.NewModal()
		modal.SetText("Mediaserver error: " + exitReason).
			AddButtons([]string{"Quit"}).
			SetBackgroundColor(tcell.ColorBlack).
			SetTextColor(tcell.ColorWhite).
			SetDoneFunc(func(int, string) {
				ui.logger.Info("closing app")
				// TODO: improve app cleanup
				done <- struct{}{}

				ui.app.Stop()
			})
		modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

		ui.pages.AddPage("modal", modal, true, true)
	})

	<-done
}

// SetState sets the state of the terminal user interface.
func (ui *UI) SetState(state domain.AppState) {
	if state.Source.ExitReason != "" {
		ui.handleMediaServerClosed(state.Source.ExitReason)
	}

	// The state is mutable so can't be passed into QueueUpdateDraw, which
	// passes it to another goroutine, without cloning it first.
	stateClone := state.Clone()
	ui.app.QueueUpdateDraw(func() {
		ui.redrawFromState(stateClone)
	})
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
	headerTracks    = "Tracks"
)

func (ui *UI) redrawFromState(state domain.AppState) {
	headerCell := func(content string, expansion int) *tview.TableCell {
		return tview.
			NewTableCell(content).
			SetExpansion(expansion).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
	}

	ui.sourceViews.url.SetText(state.Source.RTMPURL)

	tracks := dash
	if state.Source.Live && len(state.Source.Tracks) > 0 {
		tracks = strings.Join(state.Source.Tracks, ", ")
	}
	ui.sourceViews.tracks.SetText(tracks)

	if state.Source.Live {
		ui.sourceViews.status.SetText("[black:green]receiving")
	} else if state.Source.Container.State == "running" && state.Source.Container.HealthState == "healthy" {
		ui.sourceViews.status.SetText("[black:yellow]ready")
	} else {
		ui.sourceViews.status.SetText("[white:red]not ready")
	}

	ui.sourceViews.health.SetText("[white]" + cmp.Or(rightPad(state.Source.Container.HealthState, 9), dash))

	cpuPercent := dash
	if state.Source.Container.State == "running" {
		cpuPercent = fmt.Sprintf("%.1f", state.Source.Container.CPUPercent)
	}
	ui.sourceViews.cpu.SetText("[white]" + cpuPercent)

	memUsage := dash
	if state.Source.Container.State == "running" {
		memUsage = fmt.Sprintf("%.1f", float64(state.Source.Container.MemoryUsageBytes)/1024/1024)
	}
	ui.sourceViews.mem.SetText("[white]" + memUsage)

	rxRate := dash
	if state.Source.Container.State == "running" {
		rxRate = fmt.Sprintf("%d", state.Source.Container.RxRate)
	}
	ui.sourceViews.rx.SetText("[white]" + rxRate)

	ui.destView.Clear()
	ui.destView.SetCell(0, 0, headerCell("[grey]"+headerName, 3))
	ui.destView.SetCell(0, 1, headerCell("[grey]"+headerURL, 3))
	ui.destView.SetCell(0, 2, headerCell("[grey]"+headerStatus, 2))
	ui.destView.SetCell(0, 3, headerCell("[grey]"+headerContainer, 2))
	ui.destView.SetCell(0, 4, headerCell("[grey]"+headerHealth, 2))
	ui.destView.SetCell(0, 5, headerCell("[grey]"+headerCPU, 1))
	ui.destView.SetCell(0, 6, headerCell("[grey]"+headerMem, 1))
	ui.destView.SetCell(0, 7, headerCell("[grey]"+headerTx, 1))

	for i, dest := range state.Destinations {
		ui.destView.SetCell(i+1, 0, tview.NewTableCell(dest.Name))
		ui.destView.SetCell(i+1, 1, tview.NewTableCell(dest.URL).SetReference(dest.URL).SetMaxWidth(20))
		const statusLen = 10
		switch dest.Status {
		case domain.DestinationStatusLive:
			ui.destView.SetCell(
				i+1,
				2,
				tview.NewTableCell(rightPad("sending", statusLen)).
					SetTextColor(tcell.ColorBlack).
					SetBackgroundColor(tcell.ColorGreen).
					SetSelectedStyle(
						tcell.
							StyleDefault.
							Foreground(tcell.ColorBlack).
							Background(tcell.ColorGreen),
					),
			)
		default:
			ui.destView.SetCell(i+1, 2, tview.NewTableCell("[white]"+rightPad("off-air", statusLen)))
		}

		ui.destView.SetCell(i+1, 3, tview.NewTableCell("[white]"+rightPad(cmp.Or(dest.Container.State, dash), 10)))

		healthState := dash
		if dest.Status == domain.DestinationStatusLive {
			healthState = "healthy"
		}
		ui.destView.SetCell(i+1, 4, tview.NewTableCell("[white]"+rightPad(healthState, 7)))

		cpuPercent := dash
		if dest.Container.State == "running" {
			cpuPercent = fmt.Sprintf("%.1f", dest.Container.CPUPercent)
		}
		ui.destView.SetCell(i+1, 5, tview.NewTableCell("[white]"+rightPad(cpuPercent, 4)))

		memoryUsage := dash
		if dest.Container.State == "running" {
			memoryUsage = fmt.Sprintf("%.1f", float64(dest.Container.MemoryUsageBytes)/1000/1000)
		}
		ui.destView.SetCell(i+1, 6, tview.NewTableCell("[white]"+rightPad(memoryUsage, 4)))

		txRate := dash
		if dest.Container.State == "running" {
			txRate = "[white]" + rightPad(strconv.Itoa(dest.Container.TxRate), 4)
		}
		ui.destView.SetCell(i+1, 7, tview.NewTableCell(txRate))
	}
}

// Close closes the terminal user interface.
func (ui *UI) Close() {
	ui.app.Stop()
}

func (ui *UI) copySourceURLToClipboard(clipboardAvailable bool) {
	var text string
	if clipboardAvailable {
		clipboard.Write(clipboard.FmtText, []byte(ui.sourceViews.url.GetText(true)))
		text = "Ingress URL copied to clipboard"
	} else {
		text = "Copy to clipboard not available"
	}

	modal := tview.NewModal()
	modal.SetText(text).
		AddButtons([]string{"Ok"}).
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	modal.SetDoneFunc(func(buttonIndex int, _ string) {
		ui.pages.RemovePage("modal")
		ui.app.SetFocus(ui.destView)
	})

	ui.pages.AddPage("modal", modal, true, true)
}

func (ui *UI) confirmQuit() {
	modal := tview.NewModal()
	modal.SetText("Are you sure you want to quit?").
		AddButtons([]string{"Quit", "Cancel"}).
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetDoneFunc(func(buttonIndex int, _ string) {
			if buttonIndex == 1 || buttonIndex == -1 {
				ui.pages.RemovePage("modal")
				ui.app.SetFocus(ui.destView)
				return
			}

			ui.commandCh <- CommandQuit{}
		})
	modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	ui.pages.AddPage("modal", modal, true, true)
}

func (ui *UI) showAbout() {
	modal := tview.NewModal()
	modal.SetText(fmt.Sprintf(
		"%s: live stream multiplexer\n\nv0.0.0 %s (%s)",
		domain.AppName,
		ui.buildInfo.Version,
		ui.buildInfo.GoVersion,
	)).
		AddButtons([]string{"Ok"}).
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetDoneFunc(func(buttonIndex int, _ string) {
			ui.pages.RemovePage("modal")
			ui.app.SetFocus(ui.destView)
		})
	modal.SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	ui.pages.AddPage("modal", modal, true, true)
}

func rightPad(s string, n int) string {
	if s == "" || len(s) == n {
		return s
	}
	if len(s) > n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}
