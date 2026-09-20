package app

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	a.mu.Lock()
	refs := a.manager.AllPanes()
	a.mu.Unlock()

	out := make([]PaneInfo, 0, len(refs))
	for _, r := range refs {
		out = append(out, a.infoFor(r))
	}
	return out
}

// PaneInfo returns one pane's status. spec is a pane id or "active".
func (a *App) PaneInfo(spec string) (PaneInfo, error) {
	r, err := a.ref(spec)
	if err != nil {
		return PaneInfo{}, err
	}
	return a.infoFor(r), nil
}

// infoFor reads a pane's live state. It takes no lock of its own: the
// pane has its own, and reading the session tree happened in Panes/ref.
func (a *App) infoFor(r session.PaneRef) PaneInfo {
	p := r.Pane
	_, _, rows, cols := p.Rect()
	info := PaneInfo{
		Pane: p.ID, Workspace: r.Workspace, Tab: r.TabIndex, TabName: r.TabName,
		Active: r.Active, Rows: rows, Cols: cols, Cwd: p.Cwd(),
	}
	_, fg, isShell := p.Foreground()
	info.Foreground = fg
	info.Last = lastLine(p.Capture(0, false))
	info.Status, info.Reason = a.statusOf(p, fg, isShell, info.Last)
	return info
}

// statusOf applies the rules from the design: a pane is idle when its own
// shell has the terminal and it has been quiet; working while a program
// runs; and input when that program has gone quiet on a question.
func (a *App) statusOf(p *pane.Pane, fg string, isShell bool, last string) (status, reason string) {
	if p.Dead() {
		return StatusExited, "process exited"
	}
	quiet := time.Since(p.Activity())
	agent := a.cfg.Agent

	if isShell || fg == "" && quiet > agent.Settle.D() {
		if quiet < agent.Settle.D() {
			return StatusWorking, fmt.Sprintf("output %s ago", round(quiet))
		}
		return StatusIdle, fmt.Sprintf("%s, quiet %s", shellReason(fg), round(quiet))
	}
	if quiet > agent.InputAfter.D() && matchesAny(agent.Patterns, last) {
		return StatusInput, fmt.Sprintf("fg=%s, quiet %s, prompt %q", fg, round(quiet), trim(last, 40))
	}
	return StatusWorking, fmt.Sprintf("fg=%s, quiet %s", fg, round(quiet))
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

func lastLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func round(d time.Duration) time.Duration { return d.Round(100 * time.Millisecond) }

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ref resolves "active" or a pane id.
func (a *App) ref(spec string) (session.PaneRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	active := a.manager.ActivePane()
	for _, r := range a.manager.AllPanes() {
		if spec == "active" && r.Pane == active || spec == fmt.Sprint(r.Pane.ID) {
			return r, nil
		}
	}
	// Ids are never reused, so one below the high-water mark named a pane
	// that has since closed — worth its own exit code for agents.
	if id, err := strconv.Atoi(spec); err == nil && id > 0 && id <= a.manager.Issued() {
		return session.PaneRef{}, errPaneGone{id}
	}
	return session.PaneRef{}, fmt.Errorf("no pane %q", spec)
}

// ─── operations ─────────────────────────────────────────────────────────────

// NewPaneOpts describes `slat pane new`.
type NewPaneOpts struct {
	Target string // pane to split, or "active"
	Split  string // "v" (left/right) or "h" (top/bottom)
	Cwd    string // directory for the new shell; "" inherits the split pane's
	Cmd    string // command to run once it starts
	Focus  bool   // leave the new pane focused (default: restore focus)
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
	target, err := a.ref(orActive(o.Target))
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
		created.Write([]byte(o.Cmd + "\n"))
	}
	return a.PaneInfo(fmt.Sprint(created.ID))
}

// ClosePane closes a pane, as the close-pane key does.
func (a *App) ClosePane(spec string) error {
	r, err := a.ref(spec)
	if err != nil {
		return err
	}
	r.Pane.Close()
	a.markDirty()
	return nil
}

// Send writes bytes to a pane's program, as if typed.
func (a *App) Send(spec string, data []byte) error {
	r, err := a.ref(spec)
	if err != nil {
		return err
	}
	if r.Pane.Dead() {
		return errPaneGone{r.Pane.ID}
	}
	_, err = r.Pane.Write(data)
	a.markDirty()
	return err
}

// Capture returns a pane's text: the visible screen, or the scrollback
// too when history is set; n limits it to the last n lines.
func (a *App) Capture(spec string, n int, history bool) ([]string, error) {
	r, err := a.ref(spec)
	if err != nil {
		return nil, err
	}
	return r.Pane.Capture(n, history), nil
}

type errPaneGone struct{ id int }

func (e errPaneGone) Error() string { return fmt.Sprintf("pane %d is gone", e.id) }

// PaneGone reports whether err means the pane's program is no longer running.
func PaneGone(err error) bool {
	_, ok := err.(errPaneGone)
	return ok
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
		re, err := regexp.Compile(strings.TrimPrefix(s, "text="))
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
	if _, err := a.ref(spec); err != nil {
		return PaneInfo{}, false, err
	}
	tick := time.NewTicker(waitPoll)
	defer tick.Stop()
	for {
		info, err := a.PaneInfo(spec)
		if err != nil {
			// The pane was removed from the session: that is an exit.
			return PaneInfo{}, cond.Exit, err
		}
		if met, done := cond.met(a, spec, info); met || done {
			return info, met, nil
		}
		select {
		case <-ctx.Done():
			return info, false, nil
		case <-a.quit:
			return info, false, fmt.Errorf("session ended")
		case <-tick.C:
		}
	}
}

// met reports whether the condition holds, and whether waiting further is
// pointless (the pane exited while waiting for something else).
func (c WaitFor) met(a *App, spec string, info PaneInfo) (met, done bool) {
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
