package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/session"
)

// Pane statuses. See docs/design/agent-cli.md.
const (
	StatusWorking = "working" // a program has the terminal
	StatusIdle    = "idle"    // the shell is waiting for a command
	StatusInput   = "input"   // something is waiting for a human
	StatusExited  = "exited"  // the pane's program is gone
)

// PaneInfo is what `slat ls`, `slat status` and `slat wait` report.
type PaneInfo struct {
	Pane       int    `json:"pane"`
	Workspace  string `json:"workspace"`
	Tab        int    `json:"tab"`
	TabName    string `json:"tab_name"`
	Active     bool   `json:"active"`
	Status     string `json:"status"`
	Reason     string `json:"status_reason,omitempty"`
	Foreground string `json:"foreground,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	Rows       int    `json:"rows"`
	Cols       int    `json:"cols"`
	Last       string `json:"last_line,omitempty"`
}

// Panes lists every pane in the session with its current status.
func (a *App) Panes() []PaneInfo {
	return a.panes(true)
}

// PanesIn lists panes, optionally filtered by workspace and/or tab (by name or 1-based index).
func (a *App) PanesIn(wsSpec, tabSpec string) ([]PaneInfo, error) {
	if wsSpec == "" && tabSpec == "" {
		return a.panes(true), nil
	}
	a.mu.Lock()
	refs, err := a.scopedPanesLocked(wsSpec, tabSpec)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]PaneInfo, 0, len(refs))
	for _, r := range refs {
		out = append(out, a.infoFor(r, true))
	}
	return out, nil
}

// panesStatus lists every pane without querying Cwd, for periodic polling.
func (a *App) panesStatus() []PaneInfo {
	return a.panes(false)
}

func (a *App) panes(includeCwd bool) []PaneInfo {
	a.mu.Lock()
	refs := a.manager.AllPanes()
	a.mu.Unlock()

	out := make([]PaneInfo, 0, len(refs))
	for _, r := range refs {
		out = append(out, a.infoFor(r, includeCwd))
	}
	return out
}

// PaneInfo returns one pane's status. spec is a pane id or "active".
func (a *App) PaneInfo(spec string) (PaneInfo, error) {
	return a.PaneInfoTarget(PaneTarget{Pane: spec})
}

// PaneInfoTarget returns one pane's status for a scoped target.
func (a *App) PaneInfoTarget(t PaneTarget) (PaneInfo, error) {
	r, err := a.refTarget(t)
	if err != nil {
		return PaneInfo{}, err
	}
	return a.infoFor(r, true), nil
}

// infoFor reads a pane's live state. It takes no lock of its own: the
// pane has its own, and reading the session tree happened in Panes/ref.
func (a *App) infoFor(r session.PaneRef, includeCwd bool) PaneInfo {
	p := r.Pane
	_, _, rows, cols := p.Rect()
	info := PaneInfo{
		Pane: p.ID, Workspace: r.Workspace, Tab: r.TabIndex, TabName: r.TabName,
		Active: r.Active, Rows: rows, Cols: cols,
	}
	if includeCwd {
		info.Cwd = p.Cwd()
	}
	_, fg, isShell := p.Foreground()
	info.Foreground = fg
	info.Last = p.LastLine()
	info.Status, info.Reason = a.statusOf(p, fg, isShell, info.Last)
	return info
}

// statusOf applies the rules from the design: a pane is idle when its own
// shell has the terminal and it has been quiet; working while a program
// runs; and input when that program has gone quiet on a question.
func (a *App) statusOf(p *pane.Pane, fg string, isShell bool, last string) (string, string) {
	if p.Dead() {
		return StatusExited, "process exited"
	}
	quiet := time.Since(p.Activity())
	agent := a.cfg.Agent

	if isShell || fg == "" && quiet > agent.Settle.D() {
		if quiet < agent.Settle.D() {
			return StatusWorking, fmt.Sprintf("output %s ago", round(quiet))
		}
		// The shell itself can be waiting for an answer: `read -p`, or a
		// script stopped on a question. A line that ends the way prompts
		// do is the shell waiting for a command, not for a human.
		if !shellPrompt(last) && matchesAny(agent.Patterns, last) {
			return StatusInput, fmt.Sprintf("%s, quiet %s, prompt %q", shellReason(fg), round(quiet), trim(last, 40))
		}
		return StatusIdle, fmt.Sprintf("%s, quiet %s", shellReason(fg), round(quiet))
	}
	if quiet > agent.InputAfter.D() && matchesAny(agent.Patterns, last) {
		return StatusInput, fmt.Sprintf("fg=%s, quiet %s, prompt %q", fg, round(quiet), trim(last, 40))
	}
	return StatusWorking, fmt.Sprintf("fg=%s, quiet %s", fg, round(quiet))
}

// promptEnds are what shell prompts end with. A question ends with
// something else, which is how the two are told apart when the shell
// itself is in the foreground.
const promptEnds = "$#%>❯➜»λ"

func shellPrompt(line string) bool {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(line)
	return strings.ContainsRune(promptEnds, r)
}

func shellReason(fg string) string {
	if fg == "" {
		return "no program"
	}
	return "fg=" + fg
}

func matchesAny(res []*regexp.Regexp, line string) bool {
	for _, re := range res {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func round(d time.Duration) time.Duration { return d.Round(100 * time.Millisecond) }

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PaneTarget identifies a pane, optionally scoped to a workspace and/or tab
// (each by name or 1-based index).
type PaneTarget struct {
	Pane      string
	Workspace string
	Tab       string
}

func matchWorkspaceLocked(workspaces []*session.Workspace, spec string) (*session.Workspace, error) {
	if idx, err := strconv.Atoi(spec); err == nil {
		if idx >= 1 && idx <= len(workspaces) {
			return workspaces[idx-1], nil
		}
	}
	for _, ws := range workspaces {
		if ws.Name == spec {
			return ws, nil
		}
	}
	return nil, fmt.Errorf("no workspace %q", spec)
}

func matchTabLocked(ws *session.Workspace, spec string) (int, *session.Tab, error) {
	if idx, err := strconv.Atoi(spec); err == nil {
		if idx >= 1 && idx <= len(ws.Tabs) {
			return idx, ws.Tabs[idx-1], nil
		}
	}
	for i, tab := range ws.Tabs {
		if tab.Name == spec {
			return i + 1, tab, nil
		}
	}
	return 0, nil, fmt.Errorf("no tab %q in workspace %q", spec, ws.Name)
}

// scopedPanesLocked returns all PaneRefs matching wsSpec and tabSpec.
// Caller must hold a.mu.
func (a *App) scopedPanesLocked(wsSpec, tabSpec string) ([]session.PaneRef, error) {
	var workspaces []*session.Workspace
	if wsSpec != "" {
		ws, err := matchWorkspaceLocked(a.manager.Workspaces, wsSpec)
		if err != nil {
			return nil, err
		}
		workspaces = []*session.Workspace{ws}
	} else if tabSpec != "" {
		ws := a.manager.ActiveWorkspace()
		if ws == nil {
			return nil, errors.New("no active workspace")
		}
		workspaces = []*session.Workspace{ws}
	} else {
		workspaces = a.manager.Workspaces
	}

	active := a.manager.ActivePane()
	var out []session.PaneRef
	for _, ws := range workspaces {
		if tabSpec != "" {
			ti, tab, err := matchTabLocked(ws, tabSpec)
			if err != nil {
				return nil, err
			}
			for _, p := range layout.CollectPanes(tab.Layout) {
				out = append(out, session.PaneRef{
					Pane: p, Workspace: ws.Name, TabIndex: ti,
					TabName: tab.Name, Active: p == active,
				})
			}
			continue
		}
		for ti, tab := range ws.Tabs {
			for _, p := range layout.CollectPanes(tab.Layout) {
				out = append(out, session.PaneRef{
					Pane: p, Workspace: ws.Name, TabIndex: ti + 1,
					TabName: tab.Name, Active: p == active,
				})
			}
		}
	}
	return out, nil
}

func (a *App) refTarget(t PaneTarget) (session.PaneRef, error) {
	spec := orActive(t.Pane)
	a.mu.Lock()
	defer a.mu.Unlock()

	if t.Workspace == "" && t.Tab == "" {
		active := a.manager.ActivePane()
		for _, r := range a.manager.AllPanes() {
			if spec == "active" && r.Pane == active || spec == fmt.Sprint(r.Pane.ID) {
				return r, nil
			}
		}
		// Ids are never reused, so one below the high-water mark named a pane
		// that has since closed — worth its own exit code for agents.
		if id, idErr := strconv.Atoi(spec); idErr == nil && id > 0 && id <= a.manager.Issued() {
			return session.PaneRef{}, paneGoneError{id}
		}
		return session.PaneRef{}, fmt.Errorf("no pane %q", spec)
	}

	refs, err := a.scopedPanesLocked(t.Workspace, t.Tab)
	if err != nil {
		return session.PaneRef{}, err
	}
	if spec == "active" {
		var ws *session.Workspace
		if t.Workspace != "" {
			ws, _ = matchWorkspaceLocked(a.manager.Workspaces, t.Workspace)
		} else {
			ws = a.manager.ActiveWorkspace()
		}
		var targetPane *pane.Pane
		if ws != nil {
			if t.Tab != "" {
				if _, tab, tabErr := matchTabLocked(ws, t.Tab); tabErr == nil {
					targetPane = tab.ActivePane
				}
			} else if len(ws.Tabs) > 0 && ws.ActiveTabIdx >= 0 && ws.ActiveTabIdx < len(ws.Tabs) {
				targetPane = ws.Tabs[ws.ActiveTabIdx].ActivePane
			}
		}
		if targetPane != nil {
			for _, r := range refs {
				if r.Pane == targetPane {
					return r, nil
				}
			}
		}
		return session.PaneRef{}, errors.New("no active pane in target")
	}

	for _, r := range refs {
		if spec == strconv.Itoa(r.Pane.ID) {
			return r, nil
		}
	}
	if id, idErr := strconv.Atoi(spec); idErr == nil && id > 0 && id <= a.manager.Issued() {
		if a.manager.PaneByID(id) != nil {
			return session.PaneRef{}, fmt.Errorf("pane %d is not in the targeted workspace/tab", id)
		}
		return session.PaneRef{}, paneGoneError{id}
	}
	return session.PaneRef{}, fmt.Errorf("no pane %q in target", spec)
}

// ─── operations ─────────────────────────────────────────────────────────────

// NewPaneOpts describes `slat pane new`.
type NewPaneOpts struct {
	Target    string // pane to split, or "active"
	Workspace string // optional workspace name or 1-based index
	Tab       string // optional tab name or 1-based index
	Split     string // "v" (left/right) or "h" (top/bottom)
	Cwd       string // directory for the new shell; "" inherits the split pane's
	Cmd       string // command to run once it starts
	Focus     bool   // leave the new pane focused (default: restore focus)
}

// NewPane splits a pane and returns the new one.
func (a *App) NewPane(o NewPaneOpts) (PaneInfo, error) {
	dir := pane.SplitVertical
	switch o.Split {
	case "", "v", "vertical":
	case "h", "horizontal":
		dir = pane.SplitHorizontal
	default:
		return PaneInfo{}, fmt.Errorf("split must be v or h, not %q", o.Split)
	}
	target, err := a.refTarget(PaneTarget{Pane: orActive(o.Target), Workspace: o.Workspace, Tab: o.Tab})
	if err != nil {
		return PaneInfo{}, err
	}

	a.mu.Lock()
	previous := a.manager.ActivePane()
	created, err := a.manager.SplitAt(target.Pane, dir, o.Cwd)
	if err == nil {
		a.startAnim(created, dir)
		if !o.Focus && previous != nil {
			a.manager.Focus(previous)
		}
	}
	a.mu.Unlock()
	a.markDirty()
	if err != nil {
		return PaneInfo{}, err
	}

	if o.Cmd != "" {
		if _, writeErr := created.Write([]byte(o.Cmd + "\n")); writeErr != nil {
			return PaneInfo{}, writeErr
		}
	}
	return a.PaneInfo(strconv.Itoa(created.ID))
}

// ClosePane closes a pane, as the close-pane key does.
func (a *App) ClosePane(spec string) error {
	return a.ClosePaneTarget(PaneTarget{Pane: spec})
}

// ClosePaneTarget closes a scoped pane target.
func (a *App) ClosePaneTarget(t PaneTarget) error {
	r, err := a.refTarget(t)
	if err != nil {
		return err
	}
	r.Pane.Close()
	a.markDirty()
	return nil
}

// Send writes bytes to a pane's program, as if typed.
func (a *App) Send(spec string, data []byte) error {
	return a.SendTarget(PaneTarget{Pane: spec}, data)
}

// SendTarget writes bytes to a scoped pane target.
func (a *App) SendTarget(t PaneTarget, data []byte) error {
	r, err := a.refTarget(t)
	if err != nil {
		return err
	}
	if r.Pane.Dead() {
		return paneGoneError{r.Pane.ID}
	}
	_, err = r.Pane.Write(data)
	a.markDirty()
	return err
}

// Capture returns a pane's text: the visible screen, or the scrollback
// too when history is set; n limits it to the last n lines.
func (a *App) Capture(spec string, n int, history bool) ([]string, error) {
	return a.CaptureTarget(PaneTarget{Pane: spec}, n, history)
}

// CaptureTarget returns a scoped pane target's text.
func (a *App) CaptureTarget(t PaneTarget, n int, history bool) ([]string, error) {
	r, err := a.refTarget(t)
	if err != nil {
		return nil, err
	}
	return r.Pane.Capture(n, history), nil
}

// FocusDirection moves focus to the neighbouring pane in dir ("left", "down", "up", "right").
func (a *App) FocusDirection(dir string) error {
	switch dir {
	case "left", "down", "up", "right":
	default:
		return fmt.Errorf("invalid direction %q (want left, down, up, right)", dir)
	}
	a.mu.Lock()
	a.selectPane(dir)
	a.mu.Unlock()
	a.markDirty()
	return nil
}

// FocusPaneTarget moves focus to a scoped pane target across workspaces/tabs.
func (a *App) FocusPaneTarget(t PaneTarget) error {
	r, err := a.refTarget(t)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.zoomed = nil
	a.manager.Focus(r.Pane)
	a.mu.Unlock()
	a.markDirty()
	return nil
}

// SwitchSession asks an attached client of this session to switch to sock.
func (a *App) SwitchSession(sock string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.attached && sock != "" {
		a.requestSwitch(sock)
	}
}

type paneGoneError struct{ id int }

func (e paneGoneError) Error() string { return fmt.Sprintf("pane %d is gone", e.id) }

// PaneGone reports whether err means the pane's program is no longer running.
func PaneGone(err error) bool {
	var gone paneGoneError
	return errors.As(err, &gone)
}

func orActive(s string) string {
	if s == "" {
		return "active"
	}
	return s
}

// ─── wait ───────────────────────────────────────────────────────────────────

// WaitFor is a condition for Wait.
type WaitFor struct {
	Idle  bool           // the program finished (or stopped for input)
	Input bool           // the program is waiting for a human
	Exit  bool           // the pane's program ended
	Text  *regexp.Regexp // this matched the pane's recent output
}

// ParseWaitFor reads --for: idle, input, exit or text=REGEX.
func ParseWaitFor(s string) (WaitFor, error) {
	switch {
	case s == "idle":
		return WaitFor{Idle: true}, nil
	case s == "input":
		return WaitFor{Input: true}, nil
	case s == "exit":
		return WaitFor{Exit: true}, nil
	case strings.HasPrefix(s, "text="):
		pat := strings.TrimPrefix(s, "text=")
		if len(pat) > 4096 {
			return WaitFor{}, errors.New("--for text=: pattern too long (max 4096 bytes)")
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return WaitFor{}, fmt.Errorf("--for text=: %w", err)
		}
		return WaitFor{Text: re}, nil
	}
	return WaitFor{}, fmt.Errorf("--for must be idle, input, exit or text=REGEX, not %q", s)
}

// waitPoll is how often Wait re-reads the pane. Panes change on their
// program's schedule, not ours, so polling here is simpler than plumbing
// wake-ups through every caller, and cheap at this interval.
const waitPoll = 50 * time.Millisecond

// waitScan is how many lines of recent output --for text=REGEX searches.
const waitScan = 200

// Wait blocks until the condition holds, the pane exits, or ctx ends. It
// returns the pane's final state and whether the condition was met.
func (a *App) Wait(ctx context.Context, spec string, cond WaitFor) (PaneInfo, bool, error) {
	return a.WaitTarget(ctx, PaneTarget{Pane: spec}, cond)
}

// WaitTarget blocks until the condition holds for a scoped pane target.
func (a *App) WaitTarget(ctx context.Context, t PaneTarget, cond WaitFor) (PaneInfo, bool, error) {
	if _, err := a.refTarget(t); err != nil {
		return PaneInfo{}, false, err
	}
	tick := time.NewTicker(waitPoll)
	defer tick.Stop()
	for {
		info, err := a.PaneInfoTarget(t)
		if err != nil {
			// The pane was removed from the session: that is an exit.
			return PaneInfo{}, cond.Exit, err
		}
		if met, done := cond.met(a, strconv.Itoa(info.Pane), info); met || done {
			return info, met, nil
		}
		select {
		case <-ctx.Done():
			return info, false, nil
		case <-a.quit:
			return info, false, errors.New("session ended")
		case <-tick.C:
		}
	}
}

// met reports whether the condition holds, and whether waiting further is
// pointless (the pane exited while waiting for something else).
func (c WaitFor) met(a *App, spec string, info PaneInfo) (bool, bool) {
	switch {
	case c.Exit:
		return info.Status == StatusExited, false
	case info.Status == StatusExited:
		// Nothing more will happen in this pane.
		return c.Idle, true
	case c.Idle:
		// Waking on a question too: an agent waiting for a build should
		// not sleep through "overwrite? [y/N]".
		return info.Status == StatusIdle || info.Status == StatusInput, false
	case c.Input:
		return info.Status == StatusInput, false
	case c.Text != nil:
		lines, err := a.Capture(spec, waitScan, true)
		if err != nil {
			return false, true
		}
		return c.Text.MatchString(strings.Join(lines, "\n")), false
	}
	return false, true
}
