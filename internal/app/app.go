// Package app is the slat session engine: it owns the workspaces, tabs and
// panes, turns client input into actions, and paints the screen.
package app

import (
	"fmt"
	"io"
	"slices"
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

// Version is shown on the startup banner.
const Version = "v0.2.0"

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
	help     []ui.HelpEntry
	quit     chan struct{}
	quitOnce sync.Once
	detachCh chan struct{}
	dirty    chan struct{}

	mu          sync.Mutex // guards everything below
	manager     *session.Manager
	handler     *input.Handler
	screen      *ui.Screen
	cols, rows  int
	zoomed      *pane.Pane
	showHelp    bool
	prompt      *prompt
	message     string
	messageEnd  time.Time
	bannerEnd   time.Time
	bannerShown bool
	attached    bool
}

type prompt struct {
	label string
	text  []byte
	apply func(string)
}

// New creates a new App from the given config.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	a := &App{
		cfg:      cfg,
		help:     helpEntries(cfg),
		quit:     make(chan struct{}),
		detachCh: make(chan struct{}, 1),
		dirty:    make(chan struct{}, 1),
		handler:  input.NewHandler(cfg.PrefixByte, cfg.Keybinds),
		screen:   ui.NewScreen(io.Discard),
	}
	a.manager = session.NewManager(cfg.Shell, a.markDirty)
	return a, nil
}

// SetOutput sets where rendered output goes. Call it before Start.
func (a *App) SetOutput(w io.Writer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.screen = ui.NewScreen(w)
}

// Start creates the first workspace and starts rendering. It doesn't block.
func (a *App) Start(cols, rows int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cols, a.rows = max(cols, 1), max(rows, 1)
	if err := a.manager.AddWorkspace(a.paneArea()); err != nil {
		return err
	}
	go a.renderLoop()
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
		a.bannerEnd = time.Now().Add(bannerTime)
		time.AfterFunc(bannerTime, a.markDirty)
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

// Done is closed when the session has ended (quit, or the last pane died).
func (a *App) Done() <-chan struct{} { return a.quit }

// DetachRequested fires when the user asks to detach. The session keeps
// running; the host should just disconnect the client.
func (a *App) DetachRequested() <-chan struct{} { return a.detachCh }

// Shutdown terminates every pane's shell.
func (a *App) Shutdown() {
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
	area := a.paneArea()
	var borders []layout.Rect
	if a.zoomed != nil {
		visible = []*pane.Pane{a.zoomed}
		a.zoomed.SetRect(area.Row, area.Col, area.Rows, area.Cols)
	} else {
		borders = layout.Apply(a.manager.ActiveTab().Layout, area)
	}
	if !a.attached {
		return
	}

	frame := a.screen.NextFrame(a.cols, a.rows)
	now := time.Now()
	if now.Before(a.bannerEnd) {
		hint := fmt.Sprintf("press %s then ? for help", input.PrefixName(a.cfg.PrefixByte))
		ui.DrawBanner(frame, Version, hint)
		a.screen.Render(frame, ui.Cursor{}, ui.Modes{})
		return
	}

	active := a.manager.ActivePane()
	for _, p := range visible {
		p.Draw(frame.Lines)
	}
	ar, ac, arows, acols := active.Rect()
	ui.DrawBorders(frame, borders, layout.Rect{Row: ar, Col: ac, Rows: arows, Cols: acols})

	x, y, vis, style := active.Cursor()
	cur := ui.Cursor{X: ac + x, Y: ar + y, Visible: vis, Style: style}
	var modes ui.Modes
	modes.AppCursor, modes.BracketedPaste = active.Modes()

	if a.cfg.StatusBar {
		ui.DrawStatusBar(frame, a.rows-1, a.status(now))
	}
	if a.prompt != nil {
		x := ui.DrawPrompt(frame, a.rows-1, a.prompt.label, string(a.prompt.text))
		cur = ui.Cursor{X: x, Y: a.rows - 1, Visible: true}
	}
	if a.showHelp {
		ui.DrawHelp(frame, "slat · keys after "+input.PrefixName(a.cfg.PrefixByte), a.help)
		cur.Visible = false
	}
	a.screen.Render(frame, cur, modes)
}

func (a *App) status(now time.Time) ui.Status {
	ws := a.manager.ActiveWorkspace()
	st := ui.Status{
		Workspace:      ws.Name,
		WorkspaceIndex: a.manager.ActiveWorkspaceIdx,
		WorkspaceCount: len(a.manager.Workspaces),
		ActiveTab:      ws.ActiveTabIdx,
	}
	for _, t := range ws.Tabs {
		st.Tabs = append(st.Tabs, t.Name)
	}
	panes := a.manager.ActivePanes()
	st.PaneIndex = slices.Index(panes, a.manager.ActivePane())
	st.PaneCount = len(panes)
	switch {
	case a.handler.IsPrefixActive():
		st.Badge = "PREFIX"
	case a.showHelp:
		st.Badge = "HELP"
	case a.zoomed != nil:
		st.Badge = "ZOOM"
	}
	if now.Before(a.messageEnd) {
		st.Message = a.message
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
		if a.prompt != nil {
			if !a.promptKey(b) {
				return // ESC: drop the rest of the key sequence it started
			}
			continue
		}
		if a.showHelp {
			// Any key closes help. Drop the rest of the chunk so that a
			// multi-byte key (an arrow is ESC [ A) doesn't leak into the shell.
			a.showHelp = false
			return
		}

		if b == 0x1b && a.handler.IsPrefixActive() {
			// prefix + arrow key selects a pane in that direction.
			a.handler.CancelPrefix()
			if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
				if act, ok := arrowActions[buf[i+2]]; ok {
					a.do(act, b)
					i += 2
					continue
				}
			}
			return // some other escape sequence: swallow it whole
		}

		action := a.handler.ProcessByte(b)
		if action == input.ActionForwardInput {
			// Forward everything up to the next prefix byte in one write.
			j := i
			for j < len(buf) && buf[j] != a.handler.PrefixByte() {
				j++
			}
			a.forward(buf[i:j])
			i = j - 1
			continue
		}
		if !a.do(action, b) {
			return
		}
	}
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
func (a *App) do(action input.Action, b byte) bool {
	switch action {
	case input.ActionSendPrefix:
		a.forward([]byte{b})

	// ── Panes
	case input.ActionSplitVertical, input.ActionSplitHorizontal:
		dir := pane.SplitVertical
		if action == input.ActionSplitHorizontal {
			dir = pane.SplitHorizontal
		}
		a.zoomed = nil
		if err := a.manager.SplitPane(dir); err != nil {
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
	case input.ActionZoom:
		if a.zoomed != nil {
			a.zoomed = nil
		} else if len(a.manager.ActivePanes()) > 1 {
			a.zoomed = a.manager.ActivePane()
		}
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

	// ── Session
	case input.ActionShowHelp:
		a.showHelp = true
	case input.ActionDetach:
		select {
		case a.detachCh <- struct{}{}:
		default:
		}
		return false
	case input.ActionQuit:
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
	a.manager.SelectPaneInDirection(dir)
}

// ─── Prompt ─────────────────────────────────────────────────────────────────

func (a *App) startPrompt(label, current string, apply func(string)) {
	a.prompt = &prompt{label: label, text: []byte(current), apply: apply}
}

// promptKey handles one byte typed into the prompt. It returns false when
// the rest of the input chunk must be discarded.
func (a *App) promptKey(b byte) bool {
	p := a.prompt
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
					ui.HelpEntry{Key: input.PrefixName(cfg.PrefixByte), Desc: "Send prefix to shell"})
			}
		}
		if key := cfg.Keybinds[b.Name]; key != "" {
			out = append(out, ui.HelpEntry{Key: key, Desc: b.Desc})
		}
		if b.Name == "prev-tab" {
			out = append(out, ui.HelpEntry{Key: "1-9", Desc: "Go to tab N"})
		}
	}
	return out
}
