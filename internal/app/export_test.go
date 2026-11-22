package app

import (
	"maps"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

var (
	Quote       = quote
	Ago         = ago
	FuzzyMatch  = fuzzyMatch
	ShellPrompt = shellPrompt
)

type ScriptState = scriptState

// ParseMouseForTest exposes parseMouse fields for external app_test cases.
func ParseMouseForTest(b []byte, off int) (int, int, int, bool, bool, int, bool) {
	ev, n, ok := parseMouse(b, off)
	return ev.button, ev.x, ev.y, ev.press, ev.motion, n, ok
}

// EncodeMouseForTest exposes encodeMouse for external app_test cases.
func EncodeMouseForTest(button int, press, motion, shift, sgr bool, x, y int) []byte {
	return encodeMouse(mouseEvent{button: button, press: press, motion: motion, shift: shift}, sgr, x, y)
}

// NewWithConfigForTest constructs a minimal App with only cfg set.
func NewWithConfigForTest(cfg *config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) WithManagerForTest(fn func(*session.Manager)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fn(a.manager)
}

func (a *App) ScrollStateForTest() (bool, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scroll == nil {
		return false, 0
	}
	return true, a.scroll.top
}

func (a *App) PaneAreaForTest() layout.Rect {
	return a.paneArea()
}

func (a *App) DraggingForTest() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.drag != nil
}

func (a *App) SaveSessionForTest() {
	a.saveSession()
}

func (a *App) MessageForTest() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.message
}

func (a *App) ScriptOutputsForTest() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return maps.Clone(a.scriptOutputs)
}

func (a *App) StatusOfBarForTest(now time.Time) ui.Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusOfBar(now)
}

func (a *App) ShowPaneIDsForTest() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.showPaneIDs
}

func (a *App) RenderForTest() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.render()
}

func (a *App) SetNavKeysForTest(k string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Navigation.Keys = k
}

func (a *App) SetAutoRenameForTest(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.AutoRename = v
}

func (a *App) BuildTreeLabelsForTest() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	nodes := a.buildTree()
	labels := make([]string, len(nodes))
	for i, n := range nodes {
		labels[i] = n.label
	}
	return labels
}

func (a *App) HookForTest(cmd string, info PaneInfo) func() {
	return a.hook(cmd, info)
}

func (a *App) SyncTabNamesForTest(infos []PaneInfo) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.syncTabNames(infos)
}

func (a *App) ScanStatusForTest() {
	a.scanStatus()
}
