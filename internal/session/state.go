package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
)

// stateVersion is bumped when the saved shape changes. A file written by a
// different version is ignored rather than guessed at: a session that comes
// back wrong is worse than one that doesn't come back.
const stateVersion = 1

// State is a session on disk. It holds the shape of the workspaces, tabs
// and layout, where each pane was working, and what each pane had printed.
// It deliberately does not hold processes: nothing portable can restore a
// running program, so a restored pane is a fresh shell in the same
// directory, showing the output the old one left behind.
type State struct {
	Version    int              `json:"version"`
	SavedAt    time.Time        `json:"saved_at"`
	NextPane   int              `json:"next_pane"`
	Active     int              `json:"active"`
	Workspaces []WorkspaceState `json:"workspaces"`
}

type WorkspaceState struct {
	Name   string     `json:"name"`
	Active int        `json:"active"`
	Tabs   []TabState `json:"tabs"`
}

type TabState struct {
	Name   string     `json:"name"`
	Layout *NodeState `json:"layout"`
}

// NodeState mirrors layout.Node: either a pane or a split with two children.
type NodeState struct {
	Split    int           `json:"split,omitempty"`
	Ratio    float64       `json:"ratio,omitempty"`
	Pane     *PaneState    `json:"pane,omitempty"`
	Children [2]*NodeState `json:"children,omitempty"`
}

type PaneState struct {
	Cwd     string   `json:"cwd,omitempty"`
	Active  bool     `json:"active,omitempty"`
	History []string `json:"history,omitempty"`
}

// Snapshot captures the session, keeping at most history lines of each
// pane's output.
func (m *Manager) Snapshot(history int) State {
	st := State{
		Version:  stateVersion,
		SavedAt:  time.Now(),
		NextPane: m.nextPane,
		Active:   m.ActiveWorkspaceIdx,
	}
	for _, ws := range m.Workspaces {
		w := WorkspaceState{Name: ws.Name, Active: ws.ActiveTabIdx}
		for _, tab := range ws.Tabs {
			w.Tabs = append(w.Tabs, TabState{
				Name:   tab.Name,
				Layout: snapshotNode(tab.Layout, tab.ActivePane, history),
			})
		}
		st.Workspaces = append(st.Workspaces, w)
	}
	return st
}

func snapshotNode(n *layout.Node, active *pane.Pane, history int) *NodeState {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return &NodeState{Pane: &PaneState{
			Cwd:     n.Pane.Cwd(),
			Active:  n.Pane == active,
			History: n.Pane.Capture(history, true),
		}}
	}
	return &NodeState{
		Split:    int(n.Split),
		Ratio:    n.Ratio,
		Children: [2]*NodeState{snapshotNode(n.Children[0], active, history), snapshotNode(n.Children[1], active, history)},
	}
}

// Restore rebuilds a saved session: the same workspaces, tabs and layout,
// with a fresh shell per pane in the directory it was working in and the
// output it had printed already on its screen. It reports how many panes
// came back.
func (m *Manager) Restore(st State, area layout.Rect) (int, error) {
	if st.Version != stateVersion {
		return 0, fmt.Errorf("saved session is version %d, this slat writes %d", st.Version, stateVersion)
	}
	if len(st.Workspaces) == 0 {
		return 0, fmt.Errorf("saved session has no workspaces")
	}
	m.Workspaces = nil
	m.nextPane = 0
	count := 0
	for _, w := range st.Workspaces {
		ws := &Workspace{Name: w.Name, ActiveTabIdx: w.Active}
		for _, t := range w.Tabs {
			tab := &Tab{Name: t.Name}
			node, active, n, err := m.restoreNode(t.Layout, area)
			if err != nil {
				return count, err
			}
			count += n
			tab.Layout = node
			tab.ActivePane = active
			if tab.ActivePane == nil {
				tab.ActivePane = firstPane(node)
			}
			ws.Tabs = append(ws.Tabs, tab)
		}
		if len(ws.Tabs) == 0 {
			continue
		}
		ws.ActiveTabIdx = clampIdx(ws.ActiveTabIdx, len(ws.Tabs))
		m.Workspaces = append(m.Workspaces, ws)
	}
	if len(m.Workspaces) == 0 {
		return count, fmt.Errorf("saved session held no tabs")
	}
	m.ActiveWorkspaceIdx = clampIdx(st.Active, len(m.Workspaces))
	// Ids carry on from the old session so that a pane number a script
	// noted down never silently means a different pane.
	m.nextPane = max(m.nextPane, st.NextPane)
	m.nextWS = len(m.Workspaces)
	return count, nil
}

// restoreNode walks the saved tree, creating each pane at the size it will
// have once drawn, so the output put back on its screen is not reflowed by
// a resize a moment later.
func (m *Manager) restoreNode(n *NodeState, area layout.Rect) (*layout.Node, *pane.Pane, int, error) {
	if n == nil {
		return nil, nil, 0, fmt.Errorf("saved layout has an empty node")
	}
	if n.Pane != nil {
		m.nextPane++
		p, err := pane.NewRestored(m.nextPane, area.Rows, area.Cols, m.shell,
			n.Pane.Cwd, m.scrollback, n.Pane.History, m.onChange)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to start %s: %w", m.shell, err)
		}
		var active *pane.Pane
		if n.Pane.Active {
			active = p
		}
		return layout.NewLeaf(p), active, 1, nil
	}
	dir := pane.SplitDirection(n.Split)
	ratio := n.Ratio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	first, second, _ := layout.Split(area, dir, ratio)
	a, activeA, na, err := m.restoreNode(n.Children[0], first)
	if err != nil {
		return nil, nil, na, err
	}
	b, activeB, nb, err := m.restoreNode(n.Children[1], second)
	if err != nil {
		return nil, nil, na + nb, err
	}
	active := activeA
	if active == nil {
		active = activeB
	}
	return &layout.Node{Split: dir, Ratio: ratio, Children: [2]*layout.Node{a, b}}, active, na + nb, nil
}

func firstPane(n *layout.Node) *pane.Pane {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return n.Pane
	}
	if p := firstPane(n.Children[0]); p != nil {
		return p
	}
	return firstPane(n.Children[1])
}

func clampIdx(i, n int) int {
	if i < 0 || i >= n {
		return 0
	}
	return i
}

// StatePath is where the session is kept between runs. It is deliberately
// not next to the socket: $XDG_RUNTIME_DIR is cleared on reboot, which is
// the one event this file exists to survive.
func StatePath() (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "slat", "session.json"), nil
}

// SaveState writes the session, replacing any previous one atomically so a
// crash or a full disk cannot leave half a session behind.
func SaveState(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadState reads a saved session. A missing file is not an error: it only
// means there is nothing to come back to.
func LoadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, false, err
	}
	return st, st.Version == stateVersion && len(st.Workspaces) > 0, nil
}

// ClearState removes the saved session, which is what quitting means.
func ClearState(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
