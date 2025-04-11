//go:build integration

package app_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"git.netflux.io/rob/octoplex/internal/config"
	"git.netflux.io/rob/octoplex/internal/terminal"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/require"
)

func setupSimulationScreen(t *testing.T) (tcell.SimulationScreen, chan<- terminal.ScreenCapture, func() []string) {
	// Fetching the screen contents is tricky at this level of the test pyramid,
	// because we need to:
	//
	// 1. Somehow capture the screen contents, which is only available via the
	//    tcell.SimulationScreen, and...
	// 2. Do so without triggering data races.
	//
	// We can achieve this by passing a channel into the terminal actor, which
	// will send screen captures after each render. This can be stored locally
	// and asserted against when needed.
	var (
		screenCells []tcell.SimCell
		screenWidth int
		screenMu    sync.Mutex
	)

	getContents := func() []string {
		screenMu.Lock()
		defer screenMu.Unlock()

		var lines []string
		for n, _ := range screenCells {
			y := n / screenWidth

			if y > len(lines)-1 {
				lines = append(lines, "")
			}
			if len(screenCells[n].Runes) == 0 { // shouldn't really happen unless there is no output
				continue
			}
			lines[y] += string(screenCells[n].Runes[0])
		}

		return lines
	}

	t.Cleanup(func() {
		if t.Failed() {
			printScreen(getContents, "After failing")
		}
	})

	screen := tcell.NewSimulationScreen("")
	screenCaptureC := make(chan terminal.ScreenCapture, 1)
	go func() {
		for {
			select {
			case <-t.Context().Done():
				return
			case capture := <-screenCaptureC:
				screenMu.Lock()
				screenCells = capture.Cells
				screenWidth = capture.Width
				screenMu.Unlock()
			}
		}
	}()

	return screen, screenCaptureC, getContents
}

func contentsIncludes(contents []string, search string) bool {
	for _, line := range contents {
		if strings.Contains(line, search) {
			return true
		}
	}

	return false
}

func setupConfigService(t *testing.T, cfg config.Config) *config.Service {
	tmpDir, err := os.MkdirTemp("", "octoplex_"+strings.ReplaceAll(t.Name(), "/", "_"))
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	configService, err := config.NewService(func() (string, error) { return tmpDir, nil }, 1)
	require.NoError(t, err)
	require.NoError(t, configService.SetConfig(cfg))

	return configService
}

func printScreen(getContents func() []string, label string) {
	fmt.Println(label + ":")
	for _, line := range getContents() {
		fmt.Println(line)
	}
}

func sendKey(screen tcell.SimulationScreen, key tcell.Key, ch rune) {
	screen.InjectKey(key, ch, tcell.ModNone)
	time.Sleep(50 * time.Millisecond)
}

func sendKeyShift(screen tcell.SimulationScreen, key tcell.Key, ch rune) {
	screen.InjectKey(key, ch, tcell.ModShift)
	time.Sleep(50 * time.Millisecond)
}

func sendKeys(screen tcell.SimulationScreen, keys string) {
	screen.InjectKeyBytes([]byte(keys))
	time.Sleep(500 * time.Millisecond)
}

func sendBackspaces(screen tcell.SimulationScreen, n int) {
	for range n {
		screen.InjectKey(tcell.KeyBackspace, ' ', tcell.ModNone)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
}
