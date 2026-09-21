package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// watchInterval is how often every pane's status is re-read. Status is
// derived from output timing, so it changes on its own and nothing else
// would notice.
const watchInterval = 500 * time.Millisecond

// watch keeps each pane's status up to date so the status bar can mark a
// pane that needs attention, and the [agent] hooks fire once on the
// change rather than on every frame.
func (a *App) watch() {
	tick := time.NewTicker(watchInterval)
	defer tick.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-tick.C:
		}
		a.scanStatus()
	}
}

func (a *App) scanStatus() {
	infos := a.Panes()

	a.mu.Lock()
	first := a.lastStatus == nil
	if first {
		a.lastStatus = map[int]string{}
	}
	var hooks []func()
	live := make(map[int]bool, len(infos))
	attention := map[int]bool{}
	for _, info := range infos {
		live[info.Pane] = true
		if info.Status == StatusInput {
			attention[info.Pane] = true
		}
		was, known := a.lastStatus[info.Pane]
		a.lastStatus[info.Pane] = info.Status
		// Not on the first scan: panes that were already idle when the
		// daemon started never changed into it.
		if first || !known || was == info.Status {
			continue
		}
		switch info.Status {
		case StatusInput:
			hooks = append(hooks, a.hook(a.cfg.Agent.OnInput, info))
		case StatusIdle:
			hooks = append(hooks, a.hook(a.cfg.Agent.OnIdle, info))
		}
	}
	for id := range a.lastStatus {
		if !live[id] {
			delete(a.lastStatus, id)
		}
	}
	changed := len(attention) != len(a.attention)
	for id := range attention {
		if !a.attention[id] {
			changed = true
		}
	}
	a.attention = attention
	if changed {
		a.markDirty()
	}
	a.mu.Unlock()

	for _, run := range hooks {
		run()
	}
}

// hook prepares the configured command for a pane, or nil if unset. The
// returned closure is run outside the app lock: the command is somebody
// else's program and may take as long as it likes.
func (a *App) hook(cmd string, info PaneInfo) func() {
	if strings.TrimSpace(cmd) == "" {
		return func() {}
	}
	// Pane contents come from whatever is running in the pane, so every
	// substitution is shell-quoted: a last line containing a quote or a
	// semicolon is text, not more command.
	line := strings.NewReplacer(
		"%p", quote(fmt.Sprint(info.Pane)),
		"%t", quote(info.TabName),
		"%s", quote(info.Status),
		"%c", quote(info.Last),
	).Replace(cmd)

	return func() {
		c := hookShell(line)
		// Also unquoted, for hooks that would rather read the environment
		// than have values pasted into their command line.
		c.Env = append(os.Environ(),
			"SLAT_PANE="+fmt.Sprint(info.Pane),
			"SLAT_TAB="+info.TabName,
			"SLAT_STATUS="+info.Status,
			"SLAT_LINE="+info.Last)
		// The daemon's stderr is its log file; a hook's own output is not
		// allowed anywhere near the terminal.
		c.Stdout, c.Stderr = os.Stderr, os.Stderr
		if err := c.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "slat: agent hook %q: %v\n", cmd, err)
			return
		}
		go c.Wait() // reap it whenever it finishes
	}
}

// tabNeedsInput reports whether a pane in that tab is waiting for input,
// so the status bar can mark it. Called with the lock held.
func (a *App) tabNeedsInput(workspace string, tab int) bool {
	if len(a.attention) == 0 {
		return false
	}
	for _, r := range a.manager.AllPanes() {
		if r.Workspace == workspace && r.TabIndex == tab && a.attention[r.Pane.ID] {
			return true
		}
	}
	return false
}
