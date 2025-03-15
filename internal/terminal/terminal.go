package terminal

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
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

// startState represents the state of a destination from the point of view of
// the user interface: either started, starting or not started.
type startState int

const (
	startStateNotStarted startState = iota
	startStateStarting
	startStateStarted
)

// UI is responsible for managing the terminal user interface.
type UI struct {
	commandCh chan Command
	buildInfo domain.BuildInfo
	logger    *slog.Logger

	// tview state

	app            *tview.Application
	screen         tcell.Screen
	screenCaptureC chan<- ScreenCapture
	pages          *tview.Pages
	sourceViews    sourceViews
	destView       *tview.Table

	// other mutable state

	mu               sync.Mutex
	urlsToStartState map[string]startState
	allowQuit        bool
}

// Screen represents a terminal screen. This includes its desired dimensions,
// which is required to initialize the tcell.SimulationScreen.
type Screen struct {
	Screen        tcell.Screen
	Width, Height int
	CaptureC      chan<- ScreenCapture
}

// ScreenCapture represents a screen capture, which is used for integration
// testing with the tcell.SimulationScreen.
type ScreenCapture struct {
	Cells         []tcell.SimCell
	Width, Height int
}

// StartParams contains the parameters for starting a new terminal user
// interface.
type StartParams struct {
	ChanSize           int
	Logger             *slog.Logger
	ClipboardAvailable bool
	ConfigFilePath     string
	BuildInfo          domain.BuildInfo
	Screen             *Screen // Screen may be nil.
}

const defaultChanSize = 64

// StartUI starts the terminal user interface.
func StartUI(ctx context.Context, params StartParams) (*UI, error) {
	chanSize := cmp.Or(params.ChanSize, defaultChanSize)
	commandCh := make(chan Command, chanSize)

	app := tview.NewApplication()

	var screen tcell.Screen
	var screenCaptureC chan<- ScreenCapture
	if params.Screen != nil {
		screen = params.Screen.Screen
		screenCaptureC = params.Screen.CaptureC
		// Allow the tcell screen to be overridden for integration tests. If
		// params.Screen is nil, the real terminal is used.
		app.SetScreen(screen)
		// SetSize must be called after SetScreen:
		screen.SetSize(params.Screen.Width, params.Screen.Height)
	}

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
	aboutView.AddItem(tview.NewTextView().SetText("[Space] Toggle destination"), 1, 0, false)
	aboutView.AddItem(tview.NewTextView().SetText("[u] Copy ingress RTMP URL"), 1, 0, false)
	aboutView.AddItem(tview.NewTextView().SetText("[c] Copy config file path"), 1, 0, false)
	aboutView.AddItem(tview.NewTextView().SetText("[?] About"), 1, 0, false)

	sidebar.AddItem(aboutView, 0, 1, false)

	destView := tview.NewTable()
	destView.SetTitle("Egress streams")
	destView.SetBorder(true)
	destView.SetSelectable(true, false)
	destView.SetWrapSelection(true, false)
	destView.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGrey))

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
		commandCh:      commandCh,
		buildInfo:      params.BuildInfo,
		logger:         params.Logger,
		app:            app,
		screen:         screen,
		screenCaptureC: screenCaptureC,
		pages:          pages,
		sourceViews: sourceViews{
			url:    urlTextView,
			status: statusTextView,
			tracks: tracksTextView,
			health: healthTextView,
			cpu:    cpuTextView,
			mem:    memTextView,
			rx:     rxTextView,
		},
		destView:         destView,
		urlsToStartState: make(map[string]startState),
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case ' ':
				ui.toggleDestination()
			case 'u', 'U':
				ui.copySourceURLToClipboard(params.ClipboardAvailable)
			case 'c', 'C':
				ui.copyConfigFilePathToClipboard(params.ClipboardAvailable, params.ConfigFilePath)
			case '?':
				ui.showAbout()
			}
		case tcell.KeyCtrlC:
			ui.confirmQuit()
			return nil
		}

		return event
	})

	if ui.screenCaptureC != nil {
		app.SetAfterDrawFunc(ui.captureScreen)
	}

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
		ui.showModal(
			modalGroupStartupCheck,
			"Another instance of Octoplex may already be running. Pressing continue will close that instance. Continue?",
			[]string{"Continue", "Exit"},
			func(buttonIndex int, _ string) {
				if buttonIndex == 0 {
					ui.app.SetFocus(ui.destView)
					done <- true
				} else {
					done <- false
				}
			},
		)
	})

	return <-done
}

func (ui *UI) ShowDestinationErrorModal(name string, err error) {
	done := make(chan struct{})

	ui.app.QueueUpdateDraw(func() {
		ui.showModal(
			modalGroupStartupCheck,
			fmt.Sprintf(
				"Streaming to %s failed:\n\n%s",
				cmp.Or(name, "this destination"),
				err,
			),
			[]string{"Ok"},
			func(int, string) {
				done <- struct{}{}
			},
		)
	})

	<-done
}

// AllowQuit enables the quit action.
func (ui *UI) AllowQuit() {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	// This is required to prevent the user from quitting during the startup
	// check modal, when the main event loop is not yet running, and avoid an
	// unexpected user experience. It might be nice to find a way to remove this
	// but it probably means refactoring the mediaserver actor to separate
	// starting the server from starting the event loop.
	ui.allowQuit = true
}

// captureScreen captures the screen and sends it to the screenCaptureC
// channel, which must have been set in StartParams.
//
// This is required for integration testing because GetContents() must be
// called inside the tview goroutine to avoid data races.
func (ui *UI) captureScreen(screen tcell.Screen) {
	simScreen, ok := screen.(tcell.SimulationScreen)
	if !ok {
		ui.logger.Error("simulation screen not available")
	}

	cells, w, h := simScreen.GetContents()
	ui.screenCaptureC <- ScreenCapture{
		Cells:  slices.Clone(cells),
		Width:  w,
		Height: h,
	}
}

// SetState sets the state of the terminal user interface.
func (ui *UI) SetState(state domain.AppState) {
	if state.Source.ExitReason != "" {
		ui.handleMediaServerClosed(state.Source.ExitReason)
	}

	ui.mu.Lock()
	for _, dest := range state.Destinations {
		ui.urlsToStartState[dest.URL] = containerStateToStartState(dest.Container.Status)
	}
	ui.mu.Unlock()

	// The state is mutable so can't be passed into QueueUpdateDraw, which
	// passes it to another goroutine, without cloning it first.
	stateClone := state.Clone()
	ui.app.QueueUpdateDraw(func() { ui.redrawFromState(stateClone) })
}

// modalGroup represents a specific modal of which only one may be shown
// simultaneously.
type modalGroup string

const (
	modalGroupAbout        modalGroup = "about"
	modalGroupQuit         modalGroup = "quit"
	modalGroupStartupCheck modalGroup = "startup-check"
	modalGroupClipboard    modalGroup = "clipboard"
)

func (ui *UI) showModal(group modalGroup, text string, buttons []string, doneFunc func(int, string)) {
	modalName := "modal-" + string(group)
	if ui.pages.HasPage(modalName) {
		return
	}

	modal := tview.NewModal()
	modal.SetText(text).
		AddButtons(buttons).
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ui.pages.RemovePage(modalName)

			if ui.pages.GetPageCount() == 1 {
				ui.app.SetFocus(ui.destView)
			}

			if doneFunc != nil {
				doneFunc(buttonIndex, buttonLabel)
			}
		}).
		SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	ui.pages.AddPage(modalName, modal, true, true)
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
		var durStr string
		if !state.Source.LiveChangedAt.IsZero() {
			durStr = fmt.Sprintf(" (%s)", time.Since(state.Source.LiveChangedAt).Round(time.Second))
		}

		ui.sourceViews.status.SetText("[black:green]receiving" + durStr)
	} else if state.Source.Container.Status == domain.ContainerStatusRunning && state.Source.Container.HealthState == "healthy" {
		ui.sourceViews.status.SetText("[black:yellow]ready")
	} else {
		ui.sourceViews.status.SetText("[white:red]not ready")
	}

	ui.sourceViews.health.SetText("[white]" + cmp.Or(rightPad(state.Source.Container.HealthState, 9), dash))

	cpuPercent := dash
	if state.Source.Container.Status == domain.ContainerStatusRunning {
		cpuPercent = fmt.Sprintf("%.1f", state.Source.Container.CPUPercent)
	}
	ui.sourceViews.cpu.SetText("[white]" + cpuPercent)

	memUsage := dash
	if state.Source.Container.Status == domain.ContainerStatusRunning {
		memUsage = fmt.Sprintf("%.1f", float64(state.Source.Container.MemoryUsageBytes)/1024/1024)
	}
	ui.sourceViews.mem.SetText("[white]" + memUsage)

	rxRate := dash
	if state.Source.Container.Status == domain.ContainerStatusRunning {
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

		ui.destView.SetCell(i+1, 3, tview.NewTableCell("[white]"+rightPad(cmp.Or(dest.Container.Status, dash), 10)))

		healthState := dash
		if dest.Status == domain.DestinationStatusLive {
			healthState = "healthy"
		}
		ui.destView.SetCell(i+1, 4, tview.NewTableCell("[white]"+rightPad(healthState, 7)))

		cpuPercent := dash
		if dest.Container.Status == domain.ContainerStatusRunning {
			cpuPercent = fmt.Sprintf("%.1f", dest.Container.CPUPercent)
		}
		ui.destView.SetCell(i+1, 5, tview.NewTableCell("[white]"+rightPad(cpuPercent, 4)))

		memoryUsage := dash
		if dest.Container.Status == domain.ContainerStatusRunning {
			memoryUsage = fmt.Sprintf("%.1f", float64(dest.Container.MemoryUsageBytes)/1000/1000)
		}
		ui.destView.SetCell(i+1, 6, tview.NewTableCell("[white]"+rightPad(memoryUsage, 4)))

		txRate := dash
		if dest.Container.Status == domain.ContainerStatusRunning {
			txRate = "[white]" + rightPad(strconv.Itoa(dest.Container.TxRate), 4)
		}
		ui.destView.SetCell(i+1, 7, tview.NewTableCell(txRate))
	}
}

// Close closes the terminal user interface.
func (ui *UI) Close() {
	ui.app.Stop()
}

func (ui *UI) toggleDestination() {
	const urlCol = 1
	row, _ := ui.destView.GetSelection()
	url, ok := ui.destView.GetCell(row, urlCol).GetReference().(string)
	if !ok {
		return
	}

	// Communicating with the multiplexer/container client is asynchronous. To
	// ensure we can limit each destination to a single container we need some
	// kind of local mutable state which synchronously tracks the "start state"
	// of each destination.
	//
	// Something about this approach feels a tiny bit hacky. Either of these
	// approaches would be nicer, if one could be made to work:
	//
	// 1. Store the state in the *tview.Table, which would mean not recreating
	// the cells on each redraw.
	// 2. Piggy-back on the tview goroutine to handle synchronization, but that
	// seems to introduce deadlocks and/or UI bugs.
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ss := ui.urlsToStartState[url]
	switch ss {
	case startStateNotStarted:
		ui.urlsToStartState[url] = startStateStarting
		ui.commandCh <- CommandStartDestination{URL: url}
	case startStateStarting:
		// do nothing
		return
	case startStateStarted:
		ui.commandCh <- CommandStopDestination{URL: url}
	}
}

func (ui *UI) copySourceURLToClipboard(clipboardAvailable bool) {
	var text string

	url := ui.sourceViews.url.GetText(true)
	if clipboardAvailable {
		clipboard.Write(clipboard.FmtText, []byte(url))
		text = "Ingress URL copied to clipboard:\n\n" + url
	} else {
		text = "Copy to clipboard not available:\n\n" + url
	}

	ui.showModal(
		modalGroupClipboard,
		text,
		[]string{"Ok"},
		nil,
	)
}

func (ui *UI) copyConfigFilePathToClipboard(clipboardAvailable bool, configFilePath string) {
	var text string
	if clipboardAvailable {
		if configFilePath != "" {
			clipboard.Write(clipboard.FmtText, []byte(configFilePath))
			text = "Configuration file path copied to clipboard:\n\n" + configFilePath
		} else {
			text = "Configuration file path not set"
		}
	} else {
		text = "Copy to clipboard not available"
	}

	ui.showModal(
		modalGroupClipboard,
		text,
		[]string{"Ok"},
		nil,
	)
}

func (ui *UI) confirmQuit() {
	var allowQuit bool
	ui.mu.Lock()
	allowQuit = ui.allowQuit
	ui.mu.Unlock()

	if !allowQuit {
		return
	}

	ui.showModal(
		modalGroupQuit,
		"Are you sure you want to quit?",
		[]string{"Quit", "Cancel"},
		func(buttonIndex int, _ string) {
			if buttonIndex == 0 {
				ui.commandCh <- CommandQuit{}
				return
			}
		},
	)
}

func (ui *UI) showAbout() {
	ui.showModal(
		modalGroupAbout,
		fmt.Sprintf(
			"%s: live stream multiplexer\n\nv0.0.0 %s (%s)",
			domain.AppName,
			ui.buildInfo.Version,
			ui.buildInfo.GoVersion,
		),
		[]string{"Ok"},
		nil,
	)
}

// comtainerStateToStartState converts a container state to a start state.
func containerStateToStartState(containerState string) startState {
	switch containerState {
	case domain.ContainerStatusPulling, domain.ContainerStatusCreated:
		return startStateStarting
	case domain.ContainerStatusRunning, domain.ContainerStatusRestarting, domain.ContainerStatusPaused, domain.ContainerStatusRemoving:
		return startStateStarted
	default:
		return startStateNotStarted
	}
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
