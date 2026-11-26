package app

import (
	"fmt"
	"os"
	"time"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/session"
)

type sessionSig struct {
	sessionName     string
	workspaces      int
	tabs            int
	panes           int
	activeWS        int
	activeTab       int
	activePane      int
	maxActivityUnix int64
}

// saveEvery is how often a changed session is written out. It is the worst
// case for how much output a power cut can cost, traded against writing the
// file on every keystroke.
const saveEvery = 15 * time.Second

// maxSavedLines caps what is kept per pane, whatever scrollback is set to:
// a session with a few panes and a large scrollback would otherwise write
// megabytes every quarter minute.
const maxSavedLines = 2000

// restoreSession brings back the saved session, if there is one and the
// user wants it. It reports whether anything was restored; when it returns
// false the caller starts an ordinary empty session.
func (a *App) restoreSession() bool {
	if !a.cfg.Restore || a.statePath == "" {
		return false
	}
	st, ok, err := session.LoadState(a.statePath)
	if err != nil || !ok {
		return false
	}
	n, err := a.manager.Restore(st, a.paneArea())
	if err != nil || n == 0 {
		// A half-built session is worse than none: drop whatever came back
		// and let the caller start clean.
		a.manager.Shutdown()
		a.manager = session.NewManager(a.cfg.Shell, a.cfg.Scrollback, a.markDirty)
		return false
	}
	a.restoredFrom = st.SavedAt
	a.restoredPanes = n
	return true
}

// saveLoop writes the session out while it is running. Without it only a
// clean shutdown would be recoverable, and a reboot is not a clean
// shutdown -- which is the case this whole feature exists for.
func (a *App) saveLoop() {
	if !a.cfg.Restore || a.statePath == "" {
		return
	}
	t := time.NewTicker(saveEvery)
	defer t.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-t.C:
			a.saveSession()
		}
	}
}

// saveSession writes the session out now if anything changed since the last
// save. Errors are deliberately quiet: a session that cannot be saved is a
// smaller problem than a multiplexer that interrupts the user to say so, and
// the log records it.
func (a *App) saveSession() {
	if !a.cfg.Restore || a.statePath == "" {
		return
	}
	a.mu.Lock()
	sig := a.currentSessionSig()
	if sig == a.lastSavedSig {
		a.mu.Unlock()
		return
	}
	lines := min(a.cfg.Scrollback, maxSavedLines)
	st := a.manager.Snapshot(lines)
	a.mu.Unlock()
	if err := session.SaveState(a.statePath, st); err != nil {
		fmt.Fprintf(os.Stderr, "slat: save state: %v\n", err)
		return
	}
	a.mu.Lock()
	a.lastSavedSig = sig
	a.mu.Unlock()
}

func (a *App) currentSessionSig() sessionSig {
	sig := sessionSig{
		sessionName: a.manager.SessionName,
		workspaces:  len(a.manager.Workspaces),
		activeWS:    a.manager.ActiveWorkspaceIdx,
	}
	if ws := a.manager.ActiveWorkspace(); ws != nil {
		sig.activeTab = ws.ActiveTabIdx
		if ap := a.manager.ActivePane(); ap != nil {
			sig.activePane = ap.ID
		}
	}
	for _, ws := range a.manager.Workspaces {
		sig.tabs += len(ws.Tabs)
		for _, t := range ws.Tabs {
			for _, p := range layout.CollectPanes(t.Layout) {
				sig.panes++
				if act := p.Activity().UnixNano(); act > sig.maxActivityUnix {
					sig.maxActivityUnix = act
				}
			}
		}
	}
	return sig
}

// forgetSession removes the saved session. Quitting is a decision, and a
// session that came back after the user ended it would be a bug, not a
// feature.
func (a *App) forgetSession() {
	if a.statePath != "" {
		if err := session.ClearState(a.statePath); err != nil {
			fmt.Fprintf(os.Stderr, "slat: clear state: %v\n", err)
		}
	}
}

// ago renders how long ago the session was saved, in the roughest unit
// that still says something useful.
func ago(t time.Time) string {
	if t.IsZero() {
		return "an earlier session"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "moments ago"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	default:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
}
