package terminal

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.netflux.io/rob/octoplex/internal/domain"
	"git.netflux.io/rob/octoplex/internal/event"
	"git.netflux.io/rob/octoplex/internal/optional"
	"git.netflux.io/rob/octoplex/internal/shortid"
	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
	"github.com/rivo/tview"
)

type sourceViews struct {
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
	eventBus           *event.Bus
	dispatch           func(event.Command)
	clipboardAvailable bool
	buildInfo          domain.BuildInfo
	appExitC           chan error
	logger             *slog.Logger

	// tview state

	app               *tview.Application
	screen            tcell.Screen
	screenCaptureC    chan<- ScreenCapture
	pages             *tview.Pages
	container         *tview.Flex
	sourceViews       sourceViews
	destView          *tview.Table
	noDestView        *tview.TextView
	actionsView       *tview.Flex
	pullProgressModal *tview.Modal

	// other mutable state

	mu                         sync.Mutex
	destinationIDsToStartState map[uuid.UUID]startState
	rtmpURL, rtmpsURL          string

	/// addingDestination is true if add destination modal is currently visible.
	addingDestination bool
	// hasDestinations is true if the UI thinks there are destinations
	// configured.
	hasDestinations bool
	// lastSelectedDestIndex is the index of the last selected destination, starting
	// at 1 (because 0 is the header).
	lastSelectedDestIndex int
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

// Params contains the parameters for starting a new terminal user
// interface.
type Params struct {
	EventBus           *event.Bus
	Dispatcher         func(event.Command)
	Logger             *slog.Logger
	ClipboardAvailable bool
	BuildInfo          domain.BuildInfo
	Screen             *Screen // Screen may be nil.
}

// NewUI creates the user interface. Call [Run] on the *UI instance to block
// until it is completed.
func NewUI(ctx context.Context, params Params) (*UI, error) {
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
	sourceView.SetTitle("Source")
	sidebar.AddItem(sourceView, 8, 0, false)

	leftCol := tview.NewFlex()
	leftCol.SetDirection(tview.FlexRow)
	rightCol := tview.NewFlex()
	rightCol.SetDirection(tview.FlexRow)
	sourceView.AddItem(leftCol, 9, 0, false)
	sourceView.AddItem(rightCol, 0, 1, false)

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

	actionsView := tview.NewFlex()
	actionsView.SetDirection(tview.FlexRow)
	actionsView.SetBorder(true)
	actionsView.SetTitle("Actions")

	sidebar.AddItem(actionsView, 0, 1, false)

	destView := tview.NewTable()
	destView.SetTitle("Destinations")
	destView.SetBorder(true)
	destView.SetSelectable(true, false)
	destView.SetWrapSelection(true, false)
	destView.SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkSlateGrey))

	pullProgressModal := tview.NewModal()
	pullProgressModal.
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	container := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(sidebar, 40, 0, false).
		AddItem(destView, 0, 6, false)

	// noDestView is overlaid on top of the main view when there are no
	// destinations configured.
	noDestView := tview.NewTextView().
		SetText(`No destinations added yet. Press [a] to add a new destination.`).
		SetTextAlign(tview.AlignCenter).
		SetTextColor(tcell.ColorGrey)
	noDestView.SetBorder(false)

	pages := tview.NewPages()
	pages.AddPage(pageNameMain, container, true, true)
	pages.AddPage(pageNameNoDestinations, noDestView, false, false)

	app.SetRoot(pages, true)
	app.SetFocus(destView)
	app.EnableMouse(false)

	ui := &UI{
		eventBus:           params.EventBus,
		dispatch:           params.Dispatcher,
		clipboardAvailable: params.ClipboardAvailable,
		appExitC:           make(chan error, 1),
		buildInfo:          params.BuildInfo,
		logger:             params.Logger,
		app:                app,
		screen:             screen,
		screenCaptureC:     screenCaptureC,
		pages:              pages,
		container:          container,
		sourceViews: sourceViews{
			status: statusTextView,
			tracks: tracksTextView,
			health: healthTextView,
			cpu:    cpuTextView,
			mem:    memTextView,
			rx:     rxTextView,
		},
		destView:                   destView,
		noDestView:                 noDestView,
		actionsView:                actionsView,
		pullProgressModal:          pullProgressModal,
		destinationIDsToStartState: make(map[uuid.UUID]startState),
	}

	app.SetInputCapture(ui.inputCaptureHandler)
	app.SetAfterDrawFunc(ui.afterDrawHandler)

	return ui, nil
}

var ErrUserClosed = errors.New("user closed UI")

// Run runs the user interface. It always returns a non-nil error, which will
// be [ErrUserClosed] if the user voluntarily closed the UI.
func (ui *UI) Run(ctx context.Context) error {
	clientID, eventC := ui.eventBus.Register()
	defer ui.eventBus.Deregister(clientID)

	go func() {
		err := ui.app.Run()
		if err != nil {
			ui.logger.Error("Error in UI run loop, exiting", "err", err)
		}
		ui.appExitC <- err
	}()

	for {
		select {
		case evt, ok := <-eventC:
			if !ok {
				// should never happen
				return errors.New("event channel closed")
			}
			ui.app.QueueUpdateDraw(func() {
				switch evt := evt.(type) {
				case event.AppStateChangedEvent:
					ui.handleAppStateChanged(evt)
				case event.DestinationAddedEvent:
					ui.handleDestinationAdded(evt)
				case event.AddDestinationFailedEvent:
					ui.handleDestinationEventError(evt.Err)
				case event.DestinationUpdatedEvent:
					ui.handleDestinationUpdated(evt)
				case event.UpdateDestinationFailedEvent:
					ui.handleDestinationEventError(evt.Err)
				case event.StartDestinationFailedEvent:
					ui.handleStartDestinationFailed(evt)
				case event.DestinationStreamExitedEvent:
					ui.handleDestinationStreamExited(evt)
				case event.DestinationRemovedEvent:
					ui.handleDestinationRemoved(evt)
				case event.RemoveDestinationFailedEvent:
					ui.handleDestinationEventError(evt.Err)
				case event.OtherInstanceDetectedEvent:
					ui.handleOtherInstanceDetected(evt)
				case event.FatalErrorOccurredEvent:
					ui.handleFatalErrorOccurred(evt)
				default:
					// Some events are not handled by the UI, so this is normal.
					ui.logger.Debug("Unhandled event", "event", evt, "type", fmt.Sprintf("%T", evt))
				}
			})
		case <-ctx.Done():
			return ctx.Err()
		case err := <-ui.appExitC:
			return cmp.Or(err, ErrUserClosed)
		}
	}
}

func (ui *UI) inputCaptureHandler(event *tcell.EventKey) *tcell.EventKey {
	// Special case: handle CTRL-C even when a modal is visible.
	if event.Key() == tcell.KeyCtrlC {
		ui.confirmQuit()
		return nil
	}

	if ui.modalVisible() {
		return event
	}

	handleKeyUp := func() {
		row, _ := ui.destView.GetSelection()
		if row == 1 {
			ui.destView.Select(ui.destView.GetRowCount(), 0)
		}
	}

	switch event.Key() {
	case tcell.KeyRune:
		switch event.Rune() {
		case 'a', 'A':
			ui.addDestination()
			return nil
		case 'e', 'E':
			ui.updateDestination()
			return nil
		case ' ':
			ui.toggleDestination()
		case '?':
			ui.showAbout()
		case 'k': // tview vim bindings
			handleKeyUp()
		}
	case tcell.KeyF1, tcell.KeyF2:
		ui.fkeyHandler(event.Key())
	case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
		ui.removeDestination()
		return nil
	case tcell.KeyUp:
		handleKeyUp()
	}

	return event
}

func (ui *UI) fkeyHandler(key tcell.Key) {
	var urls []string
	if ui.rtmpURL != "" {
		urls = append(urls, ui.rtmpURL)
	}
	if ui.rtmpsURL != "" {
		urls = append(urls, ui.rtmpsURL)
	}

	switch key {
	case tcell.KeyF1:
		if len(urls) == 0 {
			return
		}
		ui.copySourceURLToClipboard(urls[0])
	case tcell.KeyF2:
		if len(urls) < 2 {
			return
		}
		ui.copySourceURLToClipboard(urls[1])
	}
}

func (ui *UI) handleStartDestinationFailed(event.StartDestinationFailedEvent) {
	ui.showModal(
		pageNameModalStartDestinationFailed,
		"Waiting for stream.\n\nStart streaming to a source URL then try again.",
		[]string{"Ok"},
		false,
		nil,
	)
}

func (ui *UI) handleOtherInstanceDetected(event.OtherInstanceDetectedEvent) {
	ui.showModal(
		pageNameModalStartupCheck,
		"Another instance of Octoplex may already be running.\n\nPressing continue will close that instance. Continue?",
		[]string{"Continue", "Exit"},
		false,
		func(buttonIndex int, _ string) {
			if buttonIndex == 0 {
				ui.dispatch(event.CommandCloseOtherInstance{})
			} else {
				ui.dispatch(event.CommandKillServer{})
			}
		},
	)
}

func (ui *UI) handleDestinationStreamExited(evt event.DestinationStreamExitedEvent) {
	ui.showModal(
		pageNameModalDestinationError,
		fmt.Sprintf(
			"Streaming to %s failed:\n\n%s",
			cmp.Or(evt.Name, "this destination"),
			evt.Err,
		),
		[]string{"Ok"},
		true,
		nil,
	)
}

func (ui *UI) handleFatalErrorOccurred(evt event.FatalErrorOccurredEvent) {
	ui.showModal(
		pageNameModalFatalError,
		fmt.Sprintf(
			"An error occurred:\n\n%s",
			evt.Message,
		),
		[]string{"Quit"},
		false,
		func(int, string) {
			ui.dispatch(event.CommandKillServer{})
		},
	)
}

func (ui *UI) afterDrawHandler(screen tcell.Screen) {
	if ui.screenCaptureC == nil {
		return
	}

	ui.captureScreen(screen)
}

// captureScreen captures the screen and sends it to the screenCaptureC
// channel, which must have been set in StartParams.
//
// This is required for integration testing because GetContents() must be
// called inside the tview goroutine to avoid data races.
func (ui *UI) captureScreen(screen tcell.Screen) {
	simScreen, ok := screen.(tcell.SimulationScreen)
	if !ok {
		ui.logger.Warn("captureScreen: simulation screen not available")
		return
	}

	cells, w, h := simScreen.GetContents()
	ui.screenCaptureC <- ScreenCapture{
		Cells:  slices.Clone(cells),
		Width:  w,
		Height: h,
	}
}

func (ui *UI) handleAppStateChanged(evt event.AppStateChangedEvent) {
	state := evt.State

	ui.updatePullProgress(state)

	ui.mu.Lock()
	ui.rtmpURL = state.Source.RTMPURL
	ui.rtmpsURL = state.Source.RTMPSURL

	for _, dest := range state.Destinations {
		ui.destinationIDsToStartState[dest.ID] = containerStateToStartState(dest.Container.Status)
	}

	ui.hasDestinations = len(state.Destinations) > 0
	ui.mu.Unlock()

	ui.redrawFromState(state)
}

func (ui *UI) updatePullProgress(state domain.AppState) {
	pullingContainers := make(map[string]domain.Container)

	isPulling := func(containerState string, pullPercent int) bool {
		return containerState == domain.ContainerStatusPulling && pullPercent > 0
	}

	if isPulling(state.Source.Container.Status, state.Source.Container.PullPercent) {
		pullingContainers[state.Source.Container.ImageName] = state.Source.Container
	}

	for _, dest := range state.Destinations {
		if isPulling(dest.Container.Status, dest.Container.PullPercent) {
			pullingContainers[dest.Container.ImageName] = dest.Container
		}
	}

	if len(pullingContainers) == 0 {
		ui.hideModal(pageNameModalPullProgress)
		return
	}

	// We don't really expect two images to be pulling simultaneously, but it's
	// easy enough to handle.
	imageNames := slices.Collect(maps.Keys(pullingContainers))
	slices.Sort(imageNames)
	container := pullingContainers[imageNames[0]]

	ui.updateProgressModal(container)
}

func (ui *UI) updateProgressModal(container domain.Container) {
	modalName := string(pageNameModalPullProgress)

	var status string
	// Avoid showing the long Docker pull status in the modal content.
	if len(container.PullStatus) < 30 {
		status = container.PullStatus
	}

	modalContent := fmt.Sprintf(
		"Pulling %s:\n%s (%d%%)\n\n%s",
		container.ImageName,
		status,
		container.PullPercent,
		container.PullProgress,
	)

	if ui.pages.HasPage(modalName) {
		ui.pullProgressModal.SetText(modalContent)
	} else {
		ui.pages.AddPage(modalName, ui.pullProgressModal, true, true)
	}
}

// Page names represent a specific page in the terminal user interface.
//
// Modals should generally have a unique name, which allows them to be stacked
// on top of other modals.
const (
	pageNameMain                         = "main"
	pageNameAddDestination               = "add-destination"
	pageNameUpdateDestination            = "update-destination"
	pageNameViewURLs                     = "view-urls"
	pageNameConfigUpdateFailed           = "modal-config-update-failed"
	pageNameNoDestinations               = "no-destinations"
	pageNameModalAbout                   = "modal-about"
	pageNameModalClipboard               = "modal-clipboard"
	pageNameModalDestinationError        = "modal-destination-error"
	pageNameModalFatalError              = "modal-fatal-error"
	pageNameModalPullProgress            = "modal-pull-progress"
	pageNameModalQuit                    = "modal-quit"
	pageNameModalRemoveDestination       = "modal-remove-destination"
	pageNameModalSourceError             = "modal-source-error"
	pageNameModalStartDestinationFailed  = "modal-start-destination-failed"
	pageNameModalUpdateDestinationFailed = "modal-update-destination-failed"
	pageNameModalStartupCheck            = "modal-startup-check"
)

// modalVisible returns true if any modal, including the add destination form,
// is visible.
func (ui *UI) modalVisible() bool {
	pageName, _ := ui.pages.GetFrontPage()
	return pageName != pageNameMain && pageName != pageNameNoDestinations
}

// saveSelectedDestination saves the last selected destination index to local
// mutable state.
//
// This is needed so that the user's selection can be restored
// after redrawing the screen. It may be possible to remove this if we can
// re-render the screen more selectively instead of calling [redrawFromState]
// every time the state changes.
func (ui *UI) saveSelectedDestination() {
	row, _ := ui.destView.GetSelection()
	ui.mu.Lock()
	ui.lastSelectedDestIndex = row
	ui.mu.Unlock()
}

// selectPreviousDestination sets the focus to the last-selected destination.
//
// This is needed to ensure the user ends up focused in the correct element
// after closing a modal window.
func (ui *UI) selectPreviousDestination() {
	if ui.modalVisible() {
		return
	}

	var row int
	ui.mu.Lock()
	row = ui.lastSelectedDestIndex
	ui.mu.Unlock()

	// If the last element has been removed, select the new last element.
	row = min(ui.destView.GetRowCount()-1, row)

	ui.app.SetFocus(ui.destView)

	if row == 0 {
		return
	}

	ui.destView.Select(row, 0)
}

// selectLastDestination sets the user selection to the last destination.
func (ui *UI) selectLastDestination() {
	if ui.modalVisible() {
		return
	}

	ui.app.SetFocus(ui.destView)

	if rowCount := ui.destView.GetRowCount(); rowCount > 1 {
		ui.destView.Select(rowCount-1, 0)
	}
}

func (ui *UI) showModal(
	pageName string,
	text string,
	buttons []string,
	allowMultiple bool,
	doneFunc func(int, string),
) {
	if allowMultiple {
		pageName = pageName + "-" + shortid.New().String()
	} else if ui.pages.HasPage(pageName) {
		return
	}

	modal := tview.NewModal()
	modal.SetText(text).
		AddButtons(buttons).
		SetBackgroundColor(tcell.ColorBlack).
		SetTextColor(tcell.ColorWhite).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ui.pages.RemovePage(pageName)

			if !ui.modalVisible() {
				ui.app.SetInputCapture(ui.inputCaptureHandler)
			}

			if doneFunc != nil {
				doneFunc(buttonIndex, buttonLabel)
			}

			ui.selectPreviousDestination()
		}).
		SetBorderStyle(tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite))

	ui.saveSelectedDestination()

	ui.pages.AddPage(pageName, modal, true, true)
}

func (ui *UI) hideModal(pageName string) {
	if !ui.pages.HasPage(pageName) {
		return
	}

	ui.pages.RemovePage(pageName)
	ui.app.SetFocus(ui.destView)
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

// OPTIMIZE: investigate if we could/should try to redraw less frequently
func (ui *UI) redrawFromState(state domain.AppState) {
	var addingDestination bool
	ui.mu.Lock()
	addingDestination = ui.addingDestination
	ui.mu.Unlock()

	var showNoDestinationsPage bool
	if len(state.Destinations) == 0 && !addingDestination {
		showNoDestinationsPage = true
	}

	if showNoDestinationsPage {
		x, y, w, _ := ui.destView.GetRect()
		ui.noDestView.SetRect(x+5, y+4, w-10, 3)
		ui.pages.ShowPage(pageNameNoDestinations)
	}

	headerCell := func(content string, expansion int) *tview.TableCell {
		return tview.
			NewTableCell(content).
			SetExpansion(expansion).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
	}

	// sources view

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
		ui.sourceViews.status.SetText("[black:yellow]waiting for stream")
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

	// destinations view

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
		ui.destView.SetCell(i+1, 0, tview.NewTableCell(dest.Name).SetReference(dest.ID).SetMaxWidth(20))
		ui.destView.SetCell(i+1, 1, tview.NewTableCell(dest.URL))
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

	// actions view

	ui.actionsView.Clear()
	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText("[grey]a[-]        Add destination"), 1, 0, false)
	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText("[grey]e[-]        Edit destination"), 1, 0, false)
	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText("[grey]Del[-]      Remove destination"), 1, 0, false)
	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText("[grey]Space[-]    Start/stop destination"), 1, 0, false)
	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText(""), 1, 0, false)

	i := 1
	if ui.rtmpURL != "" {
		rtmpURLView := tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf("[grey]F%d[-]       Copy source RTMP URL", i))
		ui.actionsView.AddItem(rtmpURLView, 1, 0, false)
		i++
	}

	if ui.rtmpsURL != "" {
		rtmpsURLView := tview.NewTextView().SetDynamicColors(true).SetText(fmt.Sprintf("[grey]F%d[-]       Copy source RTMPS URL", i))
		ui.actionsView.AddItem(rtmpsURLView, 1, 0, false)
	}

	ui.actionsView.AddItem(tview.NewTextView().SetDynamicColors(true).SetText("[grey]?[-]        About"), 1, 0, false)
}

// Close closes the terminal user interface.
func (ui *UI) Close() {
	ui.app.Stop()
}

// Wait waits for the terminal user interface to finish.
func (ui *UI) Wait() {
	<-ui.appExitC
}

const (
	formInputLabelName = "Name"
	formInputLabelURL  = "RTMP URL"
)

func (ui *UI) addDestination() {
	form := ui.buildDestinationForm("My stream", "rtmp://")
	form.SetTitle("Add a new destination")
	form.AddButton("Add", func() {
		ui.dispatch(event.CommandAddDestination{
			DestinationName: form.GetFormItemByLabel(formInputLabelName).(*tview.InputField).GetText(),
			URL:             form.GetFormItemByLabel(formInputLabelURL).(*tview.InputField).GetText(),
		})
	})
	form.AddButton("Cancel", func() {
		ui.closeAddDestinationForm()
		ui.selectPreviousDestination()
	})
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ui.closeAddDestinationForm()
			ui.selectPreviousDestination()
			return nil
		}
		return event
	})

	ui.mu.Lock()
	ui.addingDestination = true
	ui.mu.Unlock()

	ui.saveSelectedDestination()

	ui.pages.HidePage(pageNameNoDestinations)
	ui.pages.AddPage(pageNameAddDestination, form, false, true)
}

func (ui *UI) updateDestination() {
	row, _ := ui.destView.GetSelection()
	if row == 0 || row >= ui.destView.GetRowCount() {
		return
	}

	destinationID := ui.destView.GetCell(row, 0).GetReference().(uuid.UUID)
	var isLive bool
	ui.mu.Lock()
	isLive = ui.destinationIDsToStartState[destinationID] != startStateNotStarted
	ui.mu.Unlock()

	if isLive {
		ui.showModal(
			pageNameModalUpdateDestinationFailed,
			"You cannot update a destination while it is live.\n\nStop the live stream first.",
			[]string{"Ok"},
			false,
			nil,
		)

		return
	}

	name := ui.destView.GetCell(row, 0).Text
	url := ui.destView.GetCell(row, 1).Text

	form := ui.buildDestinationForm(name, url)
	form.SetTitle("Edit destination")
	form.AddButton("Save changes", func() {
		ui.dispatch(event.CommandUpdateDestination{
			ID:              destinationID,
			DestinationName: optional.New(form.GetFormItemByLabel(formInputLabelName).(*tview.InputField).GetText()),
			URL:             optional.New(form.GetFormItemByLabel(formInputLabelURL).(*tview.InputField).GetText()),
		})
	})
	form.AddButton("Cancel", func() {
		ui.closeUpdateDestinationForm()
		ui.selectPreviousDestination()
	})
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			ui.closeUpdateDestinationForm()
			ui.selectPreviousDestination()
			return nil
		}
		return event
	})

	ui.pages.AddPage(pageNameUpdateDestination, form, false, true)
}

func (ui *UI) buildDestinationForm(nameValue, urlValue string) *tview.Form {
	const (
		inputLen        = 60
		formInnerWidth  = inputLen + 8 + 1 // inputLen + length of longest label + one space
		formInnerHeight = 7                // line count from first input field to last button
		formWidth       = formInnerWidth + 4
		formHeight      = formInnerHeight + 2
	)

	var currWidth, currHeight int
	_, _, currWidth, currHeight = ui.container.GetRect()

	form := tview.NewForm()
	form.
		AddInputField(formInputLabelName, nameValue, inputLen, nil, nil).
		AddInputField(formInputLabelURL, urlValue, inputLen, nil, nil).
		SetFieldBackgroundColor(tcell.ColorDarkSlateGrey).
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetRect((currWidth-formWidth)/2, (currHeight-formHeight)/2, formWidth, formHeight)

	return form
}

func (ui *UI) removeDestination() {
	row, _ := ui.destView.GetSelection()
	destinationID, ok := ui.destView.GetCell(row, 0).GetReference().(uuid.UUID)
	if !ok {
		return
	}

	var started bool
	ui.mu.Lock()
	started = ui.destinationIDsToStartState[destinationID] != startStateNotStarted
	ui.mu.Unlock()

	text := "Are you sure you want to remove the destination?"
	if started {
		text += "\n\nThis will stop the current live stream for this destination."
	}

	ui.showModal(
		pageNameModalRemoveDestination,
		text,
		[]string{"Remove", "Cancel"},
		false,
		func(buttonIndex int, _ string) {
			if buttonIndex == 0 {
				ui.dispatch(event.CommandRemoveDestination{ID: destinationID, Force: true})
			}
		},
	)
}

func (ui *UI) handleDestinationAdded(event.DestinationAddedEvent) {
	ui.mu.Lock()
	ui.hasDestinations = true
	ui.mu.Unlock()

	ui.pages.HidePage(pageNameNoDestinations)
	ui.closeAddDestinationForm()
	ui.selectLastDestination()
}

func (ui *UI) handleDestinationUpdated(event.DestinationUpdatedEvent) {
	ui.closeUpdateDestinationForm()
	ui.selectPreviousDestination()
}

func (ui *UI) handleDestinationRemoved(event.DestinationRemovedEvent) {
	ui.selectPreviousDestination()
}

func (ui *UI) handleDestinationEventError(err error) {
	ui.showModal(
		pageNameConfigUpdateFailed,
		"Configuration update failed:\n\n"+err.Error(),
		[]string{"Ok"},
		false,
		func(int, string) {
			pageName, frontPage := ui.pages.GetFrontPage()
			if pageName != pageNameAddDestination && pageName != pageNameUpdateDestination {
				ui.logger.Warn("Unexpected page when configuration form closed", "page", pageName)
			}
			ui.app.SetFocus(frontPage)
		},
	)
}

func (ui *UI) closeAddDestinationForm() {
	var hasDestinations bool
	ui.mu.Lock()
	ui.addingDestination = false
	hasDestinations = ui.hasDestinations
	ui.mu.Unlock()

	ui.pages.RemovePage(pageNameAddDestination)
	if !hasDestinations {
		ui.pages.ShowPage(pageNameNoDestinations)
	}
}

func (ui *UI) closeUpdateDestinationForm() {
	ui.pages.RemovePage(pageNameUpdateDestination)
}

func (ui *UI) toggleDestination() {
	row, _ := ui.destView.GetSelection()
	destinationID, ok := ui.destView.GetCell(row, 0).GetReference().(uuid.UUID)
	if !ok {
		return
	}

	// Communicating with the replicator/container client is asynchronous. To
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

	ss := ui.destinationIDsToStartState[destinationID]
	switch ss {
	case startStateNotStarted:
		ui.destinationIDsToStartState[destinationID] = startStateStarting
		ui.dispatch(event.CommandStartDestination{ID: destinationID})
	case startStateStarting:
		// do nothing
		return
	case startStateStarted:
		ui.dispatch(event.CommandStopDestination{ID: destinationID})
	}
}

func (ui *UI) copySourceURLToClipboard(url string) {
	var text string

	if ui.clipboardAvailable {
		if err := clipboard.WriteAll(url); err != nil {
			ui.logger.Error("Error copying to clipboard", "err", err)
			text = "Unable to copy to clipboard. Select and copy instead:\n\n" + url
		} else {
			text = "URL copied to clipboard:\n\n" + url
		}
	} else {
		text = "Copy to clipboard not available:\n\n" + url
	}

	ui.showModal(
		pageNameModalClipboard,
		text,
		[]string{"Ok"},
		false,
		nil,
	)
}

func (ui *UI) confirmQuit() {
	ui.showModal(
		pageNameModalQuit,
		"Are you sure you want to quit?",
		[]string{"Quit", "Cancel"},
		false,
		func(buttonIndex int, _ string) {
			if buttonIndex == 0 {
				ui.app.Stop()
			}
		},
	)
}

func (ui *UI) showAbout() {
	commit := ui.buildInfo.Commit
	if len(commit) > 8 {
		commit = commit[:8]
	}

	ui.showModal(
		pageNameModalAbout,
		fmt.Sprintf(
			"%s: live stream replicator\n(c) Rob Watson\nhttps://git.netflux.io/rob/octoplex\n\nReleased under AGPL3.\n\nv%s (%s)\nBuilt on %s (%s).",
			domain.AppName,
			cmp.Or(ui.buildInfo.Version, "0.0.0-devel"),
			cmp.Or(commit, "unknown SHA"),
			cmp.Or(ui.buildInfo.Date, "unknown date"),
			ui.buildInfo.GoVersion,
		),
		[]string{"Ok"},
		false,
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
