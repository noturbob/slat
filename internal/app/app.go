// Package app is the slat session engine: it owns the workspaces, tabs and
// panes, turns client input into actions, and paints the screen.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

// Version is shown on the startup banner and by `slat --version`. Release
// builds set it from the git tag:
//
//	go build -ldflags "-X github.com/noturbob/slat/internal/app.Version=v1.2.3"
var Version = "dev"

const (
	frameInterval = 8 * time.Millisecond // caps redraws at ~120/s
	bannerTime    = 700 * time.Millisecond
	messageTime   = 3 * time.Second
	maxNameLen    = 32
)

// App is the slat session engine. It doesn't own a terminal: the host feeds
// it input and size changes, and it writes rendered output to the writer
// given to SetOutput. That's what lets a session outlive its client.
//
// Every exported method is safe to call from any goroutine.
type App struct {
	cfg      *config.Config
	status   ui.StatusFormat
	splash   ui.Splash
	help     []ui.HelpEntry
	quit     chan struct{}
	quitOnce sync.Once
	detachCh chan struct{}
	dirty    chan struct{}

	mu          sync.Mutex     // guards everything below
	anim        *anim          // a pane being revealed
	slide       *slide         // two panes trading places
	moving      bool           // move mode: direction keys walk the pane
	drag        *drag          // a border being pulled with the mouse
	lastStatus  map[int]string // pane id -> status, for the hooks
	attention   map[int]bool   // panes waiting for input
	manager     *session.Manager
	handler     *input.Handler
	screen      *ui.Screen
	cols, rows  int
	zoomed      *pane.Pane
	scroll      *scroll // scroll mode, or nil
	showHelp    bool
	showPaneIDs bool
	prompt      *prompt
	tree        *treeState
	message     string
	messageEnd  time.Time
	bannerEnd   time.Time
	bannerShown bool
	attached    bool

	statePath       string                // where the session is saved between runs
	sessionDir      string                // canonical working directory for this session
	listSessions    func() []SessionSummary // lists all running daemons
	listGlobalTree  func() []RemoteTreeNode
	focusRemote     func(sock string, wsIdx, tabIdx, paneID int)
	switchTarget    string // target socket for cross-session switch
	lastSavedSig    sessionSig            // skips idle 15s snapshots when nothing changed
	restoredFrom    time.Time             // when the restored session was saved
	restoredPanes   int
	statusTabs            []string
	statusAlert           []bool
	statusPaneCount       []int
	statusTabActivePane   []int
	statusTabActivePaneID []int
	scriptOutputs         map[string]string
	statusScripts         map[string]string
	preset                *session.Preset
}

type prompt struct {
	label     string
	text      []byte
	apply     func(string)
	confirm   bool
	onConfirm func()
}

// New creates a new App from the given config.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	theme, err := ui.ParseTheme(cfg.Theme)
	if err != nil {
		return nil, err
	}
	theme.Apply()
	dlg, err := ui.ParseDialogTheme(cfg.Dialog)
	if err != nil {
		return nil, err
	}
	dlg.Apply()
	fnd, err := ui.ParseFinderTheme(cfg.Finder)
	if err != nil {
		return nil, err
	}
	fnd.Apply()
	stk, err := ui.ParseStackTheme(cfg.Stack)
	if err != nil {
		return nil, err
	}
	stk.Apply()
	borders, err := ui.ParseBorders(cfg.Borders)
	if err != nil {
		return nil, err
	}
	borders.Apply()
	status, err := ui.ParseStatusConfig(cfg.Status)
	if err != nil {
		return nil, err
	}
	prefixName := cfg.PrefixKey.Name
	subtitle := "terminal multiplexer " + Version
	if cfg.Splash.HasSubtitle {
		subtitle = strings.ReplaceAll(cfg.Splash.Subtitle, "{version}", Version)
	}
	hint := fmt.Sprintf("press %s then ? for help", prefixName)
	if cfg.Splash.HasHint {
		hint = strings.ReplaceAll(cfg.Splash.Hint, "{prefix}", prefixName)
	}
	splash, err := ui.ParseSplash(cfg.Splash.Art, subtitle, hint, cfg.Splash.Fg, cfg.Splash.SubFg, cfg.Splash.Bg)
	if err != nil {
		return nil, err
	}
	sessionDir := session.CanonicalDir("")
	handler := input.NewHandler(cfg.PrefixKey, cfg.Keybinds)
	if err := handler.SetShortcuts(cfg.Shortcuts); err != nil {
		return nil, err
	}
	a := &App{
		cfg:        cfg,
		status:     status,
		splash:     splash,
		help:       helpEntries(cfg),
		quit:       make(chan struct{}),
		detachCh:   make(chan struct{}, 1),
		dirty:      make(chan struct{}, 1),
		handler:    handler,
		screen:     ui.NewScreen(io.Discard),
		sessionDir: sessionDir,
	}
	a.manager = session.NewManager(cfg.Shell, cfg.Scrollback, a.markDirty)
	if cfg.RandomSessionName {
		a.manager.SessionName = session.RandomSessionName(nil)
	} else {
		base := filepath.Base(sessionDir)
		if base == "" || base == "." || base == string(filepath.Separator) {
			base = "slat"
		}
		a.manager.SessionName = base + "-" + time.Now().Format("150405")
	}
	if cfg.Restore {
		// A path we cannot work out only means this session will not be
		// saved; it is no reason to refuse to start.
		if path, err := session.StatePathForDir(sessionDir); err == nil {
			a.statePath = path
		}
	}
	return a, nil
}

// SetOutput sets where rendered output goes. Call it before Start.
func (a *App) SetOutput(w io.Writer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.screen = ui.NewScreen(w)
}

// SetPreset configures a preconfigured session to load on Start. Call before Start.
func (a *App) SetPreset(p session.Preset) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := p
	a.preset = &cp
}

// Start creates the first workspace and starts rendering. It doesn't block.
func (a *App) Start(cols, rows int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cols, a.rows = max(cols, 1), max(rows, 1)
	switch {
	case a.preset != nil:
		if err := a.manager.ApplyPreset(*a.preset, a.paneArea()); err != nil {
			return err
		}
	case !a.restoreSession():
		if err := a.manager.AddWorkspace(a.paneArea()); err != nil {
			return err
		}
	}
	go a.renderLoop()
	go a.watch()
	go a.statusScriptLoop()
	go a.saveLoop()
	return nil
}

// Attach is called when a client connects: the new terminal's contents are
// unknown, so the next frame repaints everything. The first attach of a
// session shows the startup banner.
func (a *App) Attach(cols, rows int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cols, a.rows = max(cols, 1), max(rows, 1)
	a.attached = true
	a.screen.Invalidate()
	if !a.bannerShown {
		a.bannerShown = true
		if a.cfg.Splash.Enabled && a.cfg.Splash.Duration.D() > 0 {
			dur := a.cfg.Splash.Duration.D()
			a.bannerEnd = time.Now().Add(dur)
			time.AfterFunc(dur, a.markDirty)
		}
	}
	// Say so once, on the first attach: output that predates the reboot
	// looks like output from this one otherwise.
	if a.restoredPanes > 0 {
		a.notify(fmt.Sprintf("restored %s from %s", plural(a.restoredPanes, "pane"), ago(a.restoredFrom)))
		a.restoredPanes = 0
	}
	a.markDirty()
}

// Detach is called when the client disconnects. Panes keep running and
// are still reaped, but no frames are composed until the next Attach.
func (a *App) Detach() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attached = false
}

// HandleResize applies a new terminal size reported by the client.
func (a *App) HandleResize(cols, rows int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cols, a.rows = max(cols, 1), max(rows, 1)
	a.markDirty()
}

// SessionSummary describes a running session for `slat sessions`.
type SessionSummary struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Tabs     int    `json:"tabs"`
	Panes    int    `json:"panes"`
	Attached bool   `json:"attached"`
	Socket   string `json:"socket,omitempty"`
}

// SetSessionLister configures the callback used to discover other running
// sessions for <prefix> F and collision-free random session naming.
func (a *App) SetSessionLister(fn func() []SessionSummary) {
	a.mu.Lock()
	a.listSessions = fn
	needReroll := a.cfg.RandomSessionName && a.restoredPanes == 0
	a.mu.Unlock()
	if needReroll && fn != nil {
		sessions := fn()
		inUse := make(map[string]bool, len(sessions))
		for _, s := range sessions {
			if s.Name != "" {
				inUse[s.Name] = true
			}
		}
		a.mu.Lock()
		if inUse[a.manager.SessionName] {
			a.manager.SessionName = session.RandomSessionName(inUse)
		}
		a.mu.Unlock()
	}
}

// SetGlobalTreeLister configures the callback used to collect tree nodes from
// other running sessions for <prefix> g.
func (a *App) SetGlobalTreeLister(fn func() []RemoteTreeNode) {
	a.mu.Lock()
	a.listGlobalTree = fn
	a.mu.Unlock()
}

// SetRemoteFocuser configures the callback used to focus a workspace/tab/pane
// on another daemon before switching to it.
func (a *App) SetRemoteFocuser(fn func(sock string, wsIdx, tabIdx, paneID int)) {
	a.mu.Lock()
	a.focusRemote = fn
	a.mu.Unlock()
}

// PopSwitchTarget returns and clears any pending cross-session switch socket.
func (a *App) PopSwitchTarget() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	t := a.switchTarget
	a.switchTarget = ""
	return t
}

func (a *App) requestSwitch(sock string) {
	a.switchTarget = sock
	select {
	case a.detachCh <- struct{}{}:
	default:
	}
}

// SessionInfo returns a snapshot of the session's metadata.
func (a *App) SessionInfo() SessionSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessionInfoLocked()
}

func (a *App) sessionInfoLocked() SessionSummary {
	tabs, panes := 0, 0
	for _, ws := range a.manager.Workspaces {
		tabs += len(ws.Tabs)
		for _, t := range ws.Tabs {
			panes += layout.CountPanes(t.Layout)
		}
	}
	return SessionSummary{
		Name:     a.manager.SessionName,
		Dir:      a.sessionDir,
		Tabs:     tabs,
		Panes:    panes,
		Attached: a.attached,
	}
}

// Done is closed when the session has ended (quit, or the last pane died).
func (a *App) Done() <-chan struct{} { return a.quit }

// DetachRequested fires when the user asks to detach. The session keeps
// running; the host should just disconnect the client.
func (a *App) DetachRequested() <-chan struct{} { return a.detachCh }

// Shutdown terminates every pane's shell.
func (a *App) Shutdown() {
	// Saved before the shells are torn down, so a daemon told to stop --
	// by a reboot, or by anything else that is not the user quitting --
	// leaves the session where the next run can find it.
	a.saveSession()
	a.doQuit()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.manager.Shutdown()
}

func (a *App) doQuit() { a.quitOnce.Do(func() { close(a.quit) }) }

func (a *App) markDirty() {
	select {
	case a.dirty <- struct{}{}:
	default:
	}
}

// notify shows msg in the status bar for a few seconds.
func (a *App) notify(msg string) {
	a.message = msg
	a.messageEnd = time.Now().Add(messageTime)
	time.AfterFunc(messageTime, a.markDirty)
}

// paneArea is the screen minus the status bar row.
func (a *App) paneArea() layout.Rect {
	rows := a.rows
	if a.cfg.StatusBar && rows > 1 {
		rows--
	}
	return layout.Rect{Rows: rows, Cols: a.cols}
}

// ─── Rendering ──────────────────────────────────────────────────────────────

func (a *App) renderLoop() {
	for {
		select {
		case <-a.quit:
			return
		case <-a.dirty:
		}
		a.mu.Lock()
		a.render()
		a.mu.Unlock()
		time.Sleep(frameInterval) // let bursts of output coalesce into one frame
	}
}

// render reaps dead panes, lays out the active tab and paints one frame.
// Everything is derived from the current state on every frame, so no
// action can leave the screen out of sync by forgetting a redraw step.
func (a *App) render() {
	if a.manager.Reap() {
		a.doQuit()
		return
	}
	visible := a.manager.ActivePanes()
	if !slices.Contains(visible, a.zoomed) {
		a.zoomed = nil // closed, or on another tab
	}
	if a.scroll != nil && a.scroll.pane != a.manager.ActivePane() {
		a.scroll = nil // closed, or focus moved
	}
	area := a.paneArea()
	var borders []layout.Rect
	allPanes := visible
	tab := a.manager.ActiveTab()
	stacked := a.zoomed == nil && tab != nil && tab.Stacked && len(visible) > 1
	headerRows := 0
	if a.zoomed != nil {
		visible = []*pane.Pane{a.zoomed}
		a.zoomed.SetRect(area.Row, area.Col, area.Rows, area.Cols)
	} else if stacked {
		headerRows = min(len(visible), max(area.Rows-1, 0))
		ap := a.manager.ActivePane()
		visible = []*pane.Pane{ap}
		ap.SetRect(area.Row+headerRows, area.Col, area.Rows-headerRows, area.Cols)
	} else {
		borders = layout.Apply(tab.Layout, area)
	}
	if !a.attached {
		return
	}

	frame := a.screen.NextFrame(a.cols, a.rows)
	now := time.Now()
	if now.Before(a.bannerEnd) {
		ui.DrawSplash(frame, a.splash)
		a.screen.Render(frame, ui.Cursor{}, ui.Modes{})
		return
	}

	if a.slide != nil && !a.drawSlide(frame, now) {
		a.slide = nil
	}
	active := a.manager.ActivePane()
	for _, p := range visible {
		switch {
		case a.slide.sliding(p):
			// drawSlide painted it on its way across.
		case a.scroll != nil && p == a.scroll.pane:
			a.drawScroll(frame)
		default:
			p.Draw(frame.Lines)
		}
	}
	ar, ac, arows, acols := active.Rect()
	ui.DrawBorders(frame, borders, layout.Rect{Row: ar, Col: ac, Rows: arows, Cols: acols})
	if stacked && headerRows > 0 {
		headers := make([]ui.StackHeader, len(allPanes))
		for i, p := range allPanes {
			_, title, _ := p.Foreground()
			if title == "" {
				title = "pane"
			}
			headers[i] = ui.StackHeader{Index: i + 1, Title: title, Active: p == active}
		}
		ui.DrawStackHeaders(frame, layout.Rect{Row: area.Row, Col: area.Col, Rows: headerRows, Cols: area.Cols}, headers)
	}
	if a.anim != nil && !a.drawAnim(frame, visible, now) {
		a.anim = nil
	}
	if a.showPaneIDs {
		for _, p := range visible {
			pr, pc, _, pcols := p.Rect()
			idx := slices.Index(allPanes, p)
			ui.DrawPaneIDBadge(frame, pr, pc, pcols, p.ID, idx+1, p == active, a.zoomed != nil, a.status)
		}
	}

	x, y, vis, style := active.Cursor()
	cur := ui.Cursor{X: ac + x, Y: ar + y, Visible: vis, Style: style}
	var modes ui.Modes
	modes.AppCursor, modes.BracketedPaste = active.Modes()
	modes.Mouse = a.cfg.Mouse
	if a.scroll != nil || a.slide != nil {
		// Mid-slide the pane is painted away from its real position, and a
		// cursor left at the destination would look detached from it.
		cur.Visible = false
	}

	if a.cfg.StatusBar {
		ui.DrawStatusBar(frame, a.rows-1, a.statusOfBar(now), a.status)
	}
	if a.prompt != nil {
		x, y := ui.DrawDialog(frame, a.prompt.label, string(a.prompt.text))
		cur = ui.Cursor{X: x, Y: y, Visible: true}
	}
	if a.showHelp {
		ui.DrawHelp(frame, "slat · keys after "+a.cfg.PrefixKey.Name, a.help)
		cur.Visible = false
	}
	if a.tree != nil {
		x, y := a.drawTree(frame)
		cur = ui.Cursor{X: x, Y: y, Visible: true}
	}
	a.screen.Render(frame, cur, modes)
}

func (a *App) statusOfBar(now time.Time) ui.Status {
	ws := a.manager.ActiveWorkspace()
	st := ui.Status{
		Session:        a.manager.SessionName,
		Workspace:      ws.Name,
		WorkspaceIndex: a.manager.ActiveWorkspaceIdx,
		WorkspaceCount: len(a.manager.Workspaces),
		ActiveTab:      ws.ActiveTabIdx,
	}
	a.statusTabs = a.statusTabs[:0]
	a.statusAlert = a.statusAlert[:0]
	a.statusPaneCount = a.statusPaneCount[:0]
	a.statusTabActivePane = a.statusTabActivePane[:0]
	a.statusTabActivePaneID = a.statusTabActivePaneID[:0]
	for _, t := range ws.Tabs {
		a.statusTabs = append(a.statusTabs, t.Name)
		a.statusPaneCount = append(a.statusPaneCount, layout.CountPanes(t.Layout))
		a.statusTabActivePane = append(a.statusTabActivePane, max(layout.PaneIndex(t.Layout, t.ActivePane), 0))
		activeID := 0
		if t.ActivePane != nil {
			activeID = t.ActivePane.ID
		}
		a.statusTabActivePaneID = append(a.statusTabActivePaneID, activeID)
		alert := len(a.attention) > 0 && layout.AnyPane(t.Layout, func(p *pane.Pane) bool {
			return a.attention[p.ID]
		})
		a.statusAlert = append(a.statusAlert, alert)
	}
	st.Tabs = a.statusTabs
	st.TabAlert = a.statusAlert
	st.TabPaneCount = a.statusPaneCount
	st.TabActivePane = a.statusTabActivePane
	st.TabActivePaneID = a.statusTabActivePaneID
	panes := a.manager.ActivePanes()
	ap := a.manager.ActivePane()
	if ap != nil {
		st.PaneID = ap.ID
	}
	st.PaneIndex = slices.Index(panes, ap)
	st.PaneCount = len(panes)
	st.PrefixActive = a.handler.IsPrefixActive()
	st.Zoomed = a.zoomed != nil
	switch {
	case a.handler.IsPrefixActive():
		st.Badge = "PREFIX"
	case a.showHelp:
		st.Badge = "HELP"
	case a.scroll != nil:
		st.Badge = a.scrollBadge()
	case a.moving:
		st.Badge = "MOVE"
	case a.zoomed != nil:
		st.Badge = "ZOOM"
	case a.manager.ActiveTab() != nil && a.manager.ActiveTab().Stacked && len(panes) > 1:
		st.Badge = "STACK"
	}
	if now.Before(a.messageEnd) {
		st.Message = a.message
	}
	if len(a.scriptOutputs) > 0 {
		if a.statusScripts == nil {
			a.statusScripts = make(map[string]string, len(a.scriptOutputs))
		} else {
			clear(a.statusScripts)
		}
		for k, v := range a.scriptOutputs {
			a.statusScripts[k] = v
		}
		st.Scripts = a.statusScripts
	}
	return st
}

// ─── Input ──────────────────────────────────────────────────────────────────

// FeedInput processes raw input bytes received from the attached client.
func (a *App) FeedInput(buf []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.markDirty()

	for i := 0; i < len(buf); i++ {
		b := buf[i]
		// Mouse reports are handled wherever they land: they are not text,
		// so they must never reach a prompt, the help overlay or a shell.
		if b == 0x1b && a.cfg.Mouse {
			if _, _, ok := parseMouse(buf, i); ok {
				i = a.mouseKey(buf, i)
				continue
			}
		}
		if a.prompt != nil {
			if !a.promptKey(b) {
				return // ESC: drop the rest of the key sequence it started
			}
			continue
		}
		if a.tree != nil {
			i = a.treeKey(buf, i)
			continue
		}
		if a.showHelp {
			// Any key closes help. Drop the rest of the chunk so that a
			// multi-byte key (an arrow is ESC [ A) doesn't leak into the shell.
			a.showHelp = false
			return
		}

		if a.scroll != nil {
			if a.handler.MatchPrefix(buf[i:]) == 0 {
				i = a.scrollKey(buf, i)
				continue
			}
			a.scroll = nil // any command leaves scroll mode
		}

		if a.moving && a.handler.MatchPrefix(buf[i:]) == 0 {
			if dir, ok := moveKeys[b]; ok {
				a.movePane(dir)
				continue
			}
			if b == 0x1b && i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
				if dir, ok := arrowDirections[buf[i+2]]; ok {
					a.movePane(dir)
					i += 2
					continue
				}
			}
			// Anything else leaves move mode, and q and Esc mean only that.
			a.moving = false
			if b == 'q' || b == 0x1b {
				if b == 0x1b {
					return // drop the rest of the sequence Esc started
				}
				continue
			}
		}

		// Match multi-byte (and single-byte) prefix before the 0x1b prefix+arrow
		// handler, so pressing a multi-byte prefix twice triggers ActionSendPrefix.
		if plen := a.handler.MatchPrefix(buf[i:]); plen > 0 {
			action := a.handler.ProcessPrefix()
			i += plen - 1
			if !a.do(action) {
				return
			}
			continue
		}

		if b == 0x1b && a.handler.IsPrefixActive() {
			// prefix + arrow key selects a pane in that direction;
			// prefix + PageUp scrolls back.
			a.handler.CancelPrefix()
			if i+3 < len(buf) && string(buf[i+1:i+4]) == "[5~" {
				if a.enterScroll() {
					a.scrollBy(-max(a.scrollRows()-1, 1))
				}
				i += 3
				continue
			}
			if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
				if act, ok := arrowActions[buf[i+2]]; ok {
					a.do(act)
					i += 2
					continue
				}
			}
			return // some other escape sequence: swallow it whole
		}

		if !a.handler.IsPrefixActive() {
			if cmd, n := a.handler.MatchShortcut(buf[i:]); n > 0 {
				a.runShortcut(cmd)
				i += n - 1
				continue
			}
		}

		action := a.handler.ProcessByte(b)
		if action == input.ActionForwardInput {
			n := a.handler.ForwardLen(buf[i:])
			a.forward(buf[i : i+n])
			i += n - 1
			continue
		}
		if !a.do(action) {
			return
		}
	}
}

// arrowDirections is the arrow keys as directions, for move mode.
var arrowDirections = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left",
}

var arrowActions = map[byte]input.Action{
	'A': input.ActionSelectPaneUp,
	'B': input.ActionSelectPaneDown,
	'C': input.ActionSelectPaneRight,
	'D': input.ActionSelectPaneLeft,
}

func (a *App) forward(data []byte) {
	if p := a.manager.ActivePane(); p != nil && len(data) > 0 {
		p.Write(data)
	}
}

// do runs a command. It returns false when input processing should stop
// because the client is leaving.
func (a *App) do(action input.Action) bool {
	switch action {
	case input.ActionSendPrefix:
		a.forward(a.handler.PrefixBytes())

	// ── Panes
	case input.ActionSplitVertical, input.ActionSplitHorizontal:
		dir := pane.SplitVertical
		if action == input.ActionSplitHorizontal {
			dir = pane.SplitHorizontal
		}
		a.zoomed = nil
		if err := a.manager.SplitPane(dir); err != nil {
			a.notify(err.Error())
		} else {
			a.startAnim(a.manager.ActivePane(), dir)
		}
	case input.ActionAddPane:
		a.zoomed = nil
		if err := a.manager.AddPane(a.paneArea()); err != nil {
			a.notify(err.Error())
		}
	case input.ActionNextPane:
		a.zoomed = nil
		a.manager.NextPane()
	case input.ActionPrevPane:
		a.zoomed = nil
		a.manager.PrevPane()
	case input.ActionSelectPaneUp:
		a.selectPane("up")
	case input.ActionSelectPaneDown:
		a.selectPane("down")
	case input.ActionMoveMode:
		a.moving = !a.moving
	case input.ActionMovePaneUp:
		a.movePane("up")
	case input.ActionMovePaneDown:
		a.movePane("down")
	case input.ActionMovePaneLeft:
		a.movePane("left")
	case input.ActionMovePaneRight:
		a.movePane("right")

	case input.ActionSelectPaneLeft:
		a.selectPane("left")
	case input.ActionSelectPaneRight:
		a.selectPane("right")
	case input.ActionSwapPane:
		a.zoomed = nil
		a.manager.SwapPanes()
	case input.ActionResizeGrow:
		a.zoomed = nil
		a.manager.ResizeRatio(0.05)
	case input.ActionResizeShrink:
		a.zoomed = nil
		a.manager.ResizeRatio(-0.05)
	case input.ActionEqualizeLayout:
		a.zoomed = nil
		a.manager.EqualizeLayout()
	case input.ActionLayoutPicker:
		a.openLayoutPicker()
	case input.ActionZoom:
		if a.zoomed != nil {
			a.zoomed = nil
		} else if len(a.manager.ActivePanes()) > 1 {
			a.zoomed = a.manager.ActivePane()
		}
	case input.ActionIdentifyPanes:
		a.showPaneIDs = !a.showPaneIDs
	case input.ActionStackMode:
		a.zoomed = nil
		a.manager.ToggleStack()
	case input.ActionClosePane:
		if a.manager.KillActivePane() {
			a.doQuit()
			return false
		}

	// ── Tabs
	case input.ActionNewTab:
		if err := a.manager.CreateTab(a.paneArea()); err != nil {
			a.notify(err.Error())
		}
	case input.ActionNextTab:
		a.manager.NextTab()
	case input.ActionPrevTab:
		a.manager.PrevTab()
	case input.ActionRenameTab:
		a.startPrompt("rename tab", a.manager.ActiveTab().Name, a.manager.RenameTab)
	case input.ActionCloseTab:
		if a.manager.CloseTab() {
			a.doQuit()
			return false
		}

	// ── Workspaces
	case input.ActionNewWorkspace:
		if err := a.manager.AddWorkspace(a.paneArea()); err != nil {
			a.notify(err.Error())
		}
	case input.ActionNextWorkspace:
		a.manager.NextWorkspace()
	case input.ActionPrevWorkspace:
		a.manager.PrevWorkspace()
	case input.ActionRenameWorkspace:
		a.startPrompt("rename workspace", a.manager.ActiveWorkspace().Name, a.manager.RenameWorkspace)

	case input.ActionScrollMode:
		a.zoomed = nil
		a.enterScroll()

	// ── Session
	case input.ActionShowHelp:
		a.showHelp = true
	case input.ActionRenameSession:
		a.startPrompt("rename session", a.manager.SessionName, a.manager.RenameSession)
	case input.ActionTreePicker:
		a.openTree()
	case input.ActionSessionPicker:
		a.openSessionPicker()
	case input.ActionNextSession:
		a.switchSession(1)
	case input.ActionPrevSession:
		a.switchSession(-1)
	case input.ActionGlobalTreePicker:
		a.openGlobalTreePicker()
	case input.ActionSaveSession:
		a.startSaveSession()
	case input.ActionToggleStatusBar:
		a.cfg.StatusBar = !a.cfg.StatusBar
		a.screen.Invalidate()
	case input.ActionDetach:
		select {
		case a.detachCh <- struct{}{}:
		default:
		}
		return false
	case input.ActionQuit:
		// Quitting ends the session for good: nothing to come back to.
		a.forgetSession()
		a.doQuit()
		return false

	default:
		if action >= input.ActionGoToTab1 && action <= input.ActionGoToTab9 {
			a.manager.GoToTab(int(action - input.ActionGoToTab1))
		}
	}
	return true
}

func (a *App) selectPane(dir string) {
	a.zoomed = nil
	if tab := a.manager.ActiveTab(); tab != nil && tab.Stacked && len(a.manager.ActivePanes()) > 1 {
		switch dir {
		case "down", "right":
			a.manager.NextPane()
		case "up", "left":
			a.manager.PrevPane()
		}
		return
	}
	a.manager.SelectPaneInDirection(dir)
}

// ─── Prompt ─────────────────────────────────────────────────────────────────

func (a *App) startPrompt(label, current string, apply func(string)) {
	a.prompt = &prompt{label: label, text: []byte(current), apply: apply}
}

func (a *App) startConfirm(label string, onConfirm func()) {
	a.prompt = &prompt{label: label, confirm: true, onConfirm: onConfirm}
}

func (a *App) startSaveSession() {
	a.startPrompt("save session as", a.manager.SessionName, func(name string) {
		path, err := session.PresetPathForName(name)
		if err != nil {
			a.notify(err.Error())
			return
		}
		writePreset := func() {
			p := a.manager.ExportPreset()
			p.SessionName = name
			if err := session.SavePreset(path, p); err != nil {
				a.notify(err.Error())
				return
			}
			a.notify("saved session " + name)
		}
		if _, err := os.Stat(path); err == nil {
			a.startConfirm(fmt.Sprintf("overwrite %s.toml? [y/N]", name), writePreset)
			return
		}
		writePreset()
	})
}

// promptKey handles one byte typed into the prompt. It returns false when
// the rest of the input chunk must be discarded.
func (a *App) promptKey(b byte) bool {
	p := a.prompt
	if p.confirm {
		a.prompt = nil
		if b == 'y' || b == 'Y' {
			p.onConfirm()
			return true
		}
		if b == 0x1b {
			return false
		}
		a.notify("save canceled")
		return true
	}
	switch b {
	case '\r', '\n':
		a.prompt = nil
		if name := strings.TrimSpace(string(p.text)); name != "" {
			p.apply(name)
		}
	case 0x1b: // Esc, or the start of an arrow/function key
		a.prompt = nil
		return false
	case 0x03, 0x07: // Ctrl-C, Ctrl-G
		a.prompt = nil
	case 0x15: // Ctrl-U
		p.text = p.text[:0]
	case 0x7f, 0x08: // Backspace: remove a whole UTF-8 character
		if len(p.text) > 0 {
			_, size := utf8.DecodeLastRune(p.text)
			p.text = p.text[:len(p.text)-size]
		}
	default:
		if b >= 0x20 && utf8.RuneCount(p.text) < maxNameLen {
			p.text = append(p.text, b)
		}
	}
	return true
}

// ─── Help ───────────────────────────────────────────────────────────────────

func helpEntries(cfg *config.Config) []ui.HelpEntry {
	var out []ui.HelpEntry
	group := ""
	for _, b := range input.Bindings {
		if b.Group != group {
			group = b.Group
			out = append(out, ui.HelpEntry{Desc: group})
			if group == "Session" {
				out = append(out,
					ui.HelpEntry{Key: "?", Desc: "This help"},
					ui.HelpEntry{Key: cfg.PrefixKey.Name, Desc: "Send prefix to shell"})
			}
		}
		if key := cfg.Keybinds[b.Name]; key != "" {
			out = append(out, ui.HelpEntry{Key: key, Desc: b.Desc})
		}
		if b.Name == "prev-tab" {
			out = append(out, ui.HelpEntry{Key: "1-9", Desc: "Go to tab N"})
		}
	}
	if len(cfg.Shortcuts) > 0 {
		out = append(out, ui.HelpEntry{Desc: "Shortcuts"})
		keys := make([]string, 0, len(cfg.Shortcuts))
		for k := range cfg.Shortcuts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, ui.HelpEntry{Key: k, Desc: cfg.Shortcuts[k]})
		}
	}
	return out
}
