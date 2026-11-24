package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/ui"
)

const (
	statusScriptTick       = 250 * time.Millisecond
	statusScriptMaxOutByte = 4096
	scriptMaxErrBytes      = 1024
)

type scriptPaneState struct {
	ID     int  `json:"id"`
	Index  int  `json:"index"`
	Active bool `json:"active"`
	Rows   int  `json:"rows"`
	Cols   int  `json:"cols"`
	Dead   bool `json:"dead"`
}

type scriptTabState struct {
	Index  int               `json:"index"`
	Name   string            `json:"name"`
	Active bool              `json:"active"`
	Panes  []scriptPaneState `json:"panes"`
}

type scriptWorkspaceState struct {
	Index  int              `json:"index"`
	Name   string           `json:"name"`
	Active bool             `json:"active"`
	Tabs   []scriptTabState `json:"tabs"`
}

type scriptState struct {
	Session         string                 `json:"session"`
	SessionDir      string                 `json:"session_dir"`
	ActiveWorkspace int                    `json:"active_workspace"`
	Workspaces      []scriptWorkspaceState `json:"workspaces"`
}

type scriptSnapshot struct {
	sessionName string
	sessionDir  string
	wsName      string
	wsIdx       int // 1-based, 0 if none
	wsCount     int
	tabName     string
	tabIdx      int // 1-based, 0 if none
	tabCount    int
	paneID      int
	paneIdx     int // 1-based, 0 if none
	paneCount   int
	paneRows    int
	paneCols    int
	zoomed      bool
	activePane  *pane.Pane
	stateJSON   string
}

// snapshotScriptContextLocked captures session/workspace/tab/pane details.
// Caller must hold a.mu.
func (a *App) snapshotScriptContextLocked(includeState bool) scriptSnapshot {
	snap := scriptSnapshot{
		sessionName: a.manager.SessionName,
		sessionDir:  a.sessionDir,
		wsCount:     len(a.manager.Workspaces),
		zoomed:      a.zoomedPane() != nil,
	}
	if ws := a.manager.ActiveWorkspace(); ws != nil {
		snap.wsName = ws.Name
		snap.wsIdx = a.manager.ActiveWorkspaceIdx + 1
		snap.tabCount = len(ws.Tabs)
		if tab := a.manager.ActiveTab(); tab != nil {
			snap.tabName = tab.Name
			snap.tabIdx = ws.ActiveTabIdx + 1
			panes := a.manager.ActivePanes()
			snap.paneCount = len(panes)
			if p := a.manager.ActivePane(); p != nil {
				snap.activePane = p
				snap.paneID = p.ID
				if idx := slices.Index(panes, p); idx >= 0 {
					snap.paneIdx = idx + 1
				}
				_, _, snap.paneRows, snap.paneCols = p.Rect()
			}
		}
	}
	if includeState {
		state := scriptState{
			Session:         a.manager.SessionName,
			SessionDir:      a.sessionDir,
			ActiveWorkspace: a.manager.ActiveWorkspaceIdx + 1,
			Workspaces:      make([]scriptWorkspaceState, 0, len(a.manager.Workspaces)),
		}
		for wi, ws := range a.manager.Workspaces {
			wsState := scriptWorkspaceState{
				Index:  wi + 1,
				Name:   ws.Name,
				Active: wi == a.manager.ActiveWorkspaceIdx,
				Tabs:   make([]scriptTabState, 0, len(ws.Tabs)),
			}
			for ti, tab := range ws.Tabs {
				panes := layout.CollectPanes(tab.Layout)
				tabState := scriptTabState{
					Index:  ti + 1,
					Name:   tab.Name,
					Active: wi == a.manager.ActiveWorkspaceIdx && ti == ws.ActiveTabIdx,
					Panes:  make([]scriptPaneState, 0, len(panes)),
				}
				for pi, p := range panes {
					_, _, rows, cols := p.Rect()
					tabState.Panes = append(tabState.Panes, scriptPaneState{
						ID:     p.ID,
						Index:  pi + 1,
						Active: p == tab.ActivePane,
						Rows:   rows,
						Cols:   cols,
						Dead:   p.Dead(),
					})
				}
				wsState.Tabs = append(wsState.Tabs, tabState)
			}
			state.Workspaces = append(state.Workspaces, wsState)
		}
		if b, err := json.Marshal(state); err == nil {
			snap.stateJSON = string(b)
		}
	}
	return snap
}

func (a *App) snapshotScriptContext(includeState bool) scriptSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotScriptContextLocked(includeState)
}

func buildScriptEnv(snap scriptSnapshot) []string {
	zoomed := "0"
	if snap.zoomed {
		zoomed = "1"
	}
	var paneCwd, paneFg string
	var panePID int
	if snap.activePane != nil {
		paneCwd = snap.activePane.Cwd()
		panePID, paneFg, _ = snap.activePane.Foreground()
	}
	env := append(os.Environ(),
		"SLAT_SESSION="+snap.sessionName,
		"SLAT_SESSION_DIR="+snap.sessionDir,
		"SLAT_WORKSPACE="+snap.wsName,
		"SLAT_WORKSPACE_INDEX="+strconv.Itoa(snap.wsIdx),
		"SLAT_WORKSPACE_COUNT="+strconv.Itoa(snap.wsCount),
		"SLAT_TAB="+snap.tabName,
		"SLAT_TAB_INDEX="+strconv.Itoa(snap.tabIdx),
		"SLAT_TAB_COUNT="+strconv.Itoa(snap.tabCount),
		"SLAT_PANE="+strconv.Itoa(snap.paneID),
		"SLAT_PANE_INDEX="+strconv.Itoa(snap.paneIdx),
		"SLAT_PANE_COUNT="+strconv.Itoa(snap.paneCount),
		"SLAT_PANE_CWD="+paneCwd,
		"SLAT_PANE_FG="+paneFg,
		"SLAT_PANE_PID="+strconv.Itoa(panePID),
		"SLAT_PANE_ROWS="+strconv.Itoa(snap.paneRows),
		"SLAT_PANE_COLS="+strconv.Itoa(snap.paneCols),
		"SLAT_PANE_ZOOMED="+zoomed,
	)
	if snap.stateJSON != "" {
		env = append(env, "SLAT_STATE_JSON="+snap.stateJSON)
	}
	return env
}

// limitedBuffer caps captured command output in memory.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	rem := l.max - l.buf.Len()
	if rem > 0 {
		if len(p) > rem {
			l.buf.Write(p[:rem])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

// runShortcut executes a configured shortcut command asynchronously with SLAT_*
// context. Caller must hold a.mu.
func (a *App) runShortcut(cmd string) {
	snap := a.snapshotScriptContextLocked(true)
	select {
	case hookSem <- struct{}{}:
	default:
		a.notify("shortcut skipped: too many running scripts")
		return
	}
	go func() {
		defer func() { <-hookSem }()
		ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
		defer cancel()

		c := hookShellContext(ctx, cmd)
		c.Env = buildScriptEnv(snap)
		var stderr limitedBuffer
		stderr.max = scriptMaxErrBytes
		c.Stdout = os.Stderr
		c.Stderr = &stderr

		if err := c.Run(); err != nil {
			errMsg := strings.TrimSpace(stderr.buf.String())
			if errMsg != "" {
				fmt.Fprintf(os.Stderr, "slat: shortcut %q: %v: %s\n", cmd, err, errMsg)
			} else {
				fmt.Fprintf(os.Stderr, "slat: shortcut %q: %v\n", cmd, err)
			}
			a.mu.Lock()
			a.notify(fmt.Sprintf("shortcut failed: %v", err))
			a.mu.Unlock()
		}
	}()
}

type statusScriptResult struct {
	name   string
	output string
}

// statusScriptLoop runs configured [status.scripts.*] commands on their
// intervals and updates cached status bar outputs without blocking rendering.
func (a *App) statusScriptLoop() {
	if len(a.status.Scripts) == 0 {
		return
	}
	nextRun := make(map[string]time.Time, len(a.status.Scripts))
	running := make(map[string]bool, len(a.status.Scripts))
	now := time.Now()
	for name := range a.status.Scripts {
		nextRun[name] = now
	}

	doneCh := make(chan statusScriptResult, len(a.status.Scripts))
	tick := time.NewTicker(statusScriptTick)
	defer tick.Stop()

	launchDue := func(now time.Time) {
		for name, spec := range a.status.Scripts {
			if running[name] || now.Before(nextRun[name]) {
				continue
			}
			running[name] = true
			if spec.Interval == 0 {
				// Run once.
				nextRun[name] = time.Unix(1<<62, 0)
			} else {
				nextRun[name] = now.Add(spec.Interval)
			}
			go func(name string, spec ui.StatusScriptSpec) {
				out := a.execStatusScript(name, spec)
				select {
				case doneCh <- statusScriptResult{name: name, output: out}:
				case <-a.quit:
				}
			}(name, spec)
		}
	}

	launchDue(now)
	for {
		select {
		case <-a.quit:
			return
		case res := <-doneCh:
			running[res.name] = false
			a.mu.Lock()
			if a.scriptOutputs == nil {
				a.scriptOutputs = make(map[string]string, len(a.status.Scripts))
			}
			prev, ok := a.scriptOutputs[res.name]
			if !ok || prev != res.output {
				a.scriptOutputs[res.name] = res.output
				a.markDirty()
			}
			a.mu.Unlock()
		case t := <-tick.C:
			launchDue(t)
		}
	}
}

func (a *App) execStatusScript(name string, spec ui.StatusScriptSpec) string {
	snap := a.snapshotScriptContext(false)
	ctx, cancel := context.WithTimeout(context.Background(), spec.Timeout)
	defer cancel()

	c := hookShellContext(ctx, spec.Command)
	c.Env = buildScriptEnv(snap)
	var stdout, stderr limitedBuffer
	stdout.max = statusScriptMaxOutByte
	stderr.max = scriptMaxErrBytes
	c.Stdout = &stdout
	c.Stderr = &stderr

	if err := c.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.buf.String())
		if errMsg != "" {
			fmt.Fprintf(os.Stderr, "slat: status script %q: %v: %s\n", name, err, errMsg)
		} else {
			fmt.Fprintf(os.Stderr, "slat: status script %q: %v\n", name, err)
		}
		return spec.Fallback
	}
	return strings.TrimRight(stdout.buf.String(), "\r\n")
}
