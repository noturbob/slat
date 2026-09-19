// Package session holds the workspace / tab / pane tree.
//
// Manager is not safe for concurrent use: app.App serializes every call
// behind its own mutex.
package session

import (
	"fmt"
	"slices"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
)

// Tab holds a layout tree and an active pane pointer.
type Tab struct {
	Name       string
	Layout     *layout.Node
	ActivePane *pane.Pane
}

// Workspace groups tabs under a named context.
type Workspace struct {
	Name         string
	Tabs         []*Tab
	ActiveTabIdx int
}

// Manager manages all workspaces, tabs, and panes.
type Manager struct {
	Workspaces         []*Workspace
	ActiveWorkspaceIdx int

	shell    string
	onChange func()
	nextPane int
	nextWS   int
}

// NewManager creates a session manager whose panes run shell; onChange is
// passed through to pane.New.
func NewManager(shell string, onChange func()) *Manager {
	return &Manager{shell: shell, onChange: onChange}
}

func (m *Manager) newPane(rows, cols int, dir string) (*pane.Pane, error) {
	m.nextPane++
	p, err := pane.New(m.nextPane, rows, cols, m.shell, dir, m.onChange)
	if err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", m.shell, err)
	}
	return p, nil
}

// activeDir is the directory new panes start in: the active pane's.
func (m *Manager) activeDir() string {
	if p := m.ActivePane(); p != nil {
		return p.Cwd()
	}
	return ""
}

// ─── Workspace ──────────────────────────────────────────────────────────────

// AddWorkspace creates a new workspace with one tab and switches to it.
func (m *Manager) AddWorkspace(area layout.Rect) error {
	m.nextWS++
	name := "main"
	if m.nextWS > 1 {
		name = fmt.Sprintf("ws-%d", m.nextWS)
	}
	ws := &Workspace{Name: name}
	dir := m.activeDir()
	m.Workspaces = append(m.Workspaces, ws)
	prev := m.ActiveWorkspaceIdx
	m.ActiveWorkspaceIdx = len(m.Workspaces) - 1
	if err := m.createTab(area, dir); err != nil {
		m.Workspaces = m.Workspaces[:len(m.Workspaces)-1]
		m.ActiveWorkspaceIdx = prev
		return err
	}
	return nil
}

func (m *Manager) NextWorkspace() { m.cycleWorkspace(1) }
func (m *Manager) PrevWorkspace() { m.cycleWorkspace(-1) }

func (m *Manager) cycleWorkspace(d int) {
	if n := len(m.Workspaces); n > 0 {
		m.ActiveWorkspaceIdx = (m.ActiveWorkspaceIdx + d + n) % n
	}
}

// RenameWorkspace renames the active workspace.
func (m *Manager) RenameWorkspace(name string) {
	if ws := m.ActiveWorkspace(); ws != nil {
		ws.Name = name
	}
}

// ActiveWorkspace returns the currently active workspace.
func (m *Manager) ActiveWorkspace() *Workspace {
	if len(m.Workspaces) == 0 {
		return nil
	}
	return m.Workspaces[m.ActiveWorkspaceIdx]
}

// ─── Tab ────────────────────────────────────────────────────────────────────

// CreateTab creates a new tab in the current workspace and switches to it.
func (m *Manager) CreateTab(area layout.Rect) error {
	return m.createTab(area, m.activeDir())
}

func (m *Manager) createTab(area layout.Rect, dir string) error {
	ws := m.ActiveWorkspace()
	p, err := m.newPane(area.Rows, area.Cols, dir)
	if err != nil {
		return err
	}
	ws.Tabs = append(ws.Tabs, &Tab{
		Name:       "shell",
		Layout:     layout.NewLeaf(p),
		ActivePane: p,
	})
	ws.ActiveTabIdx = len(ws.Tabs) - 1
	return nil
}

func (m *Manager) NextTab() { m.cycleTab(1) }
func (m *Manager) PrevTab() { m.cycleTab(-1) }

func (m *Manager) cycleTab(d int) {
	if ws := m.ActiveWorkspace(); ws != nil && len(ws.Tabs) > 0 {
		n := len(ws.Tabs)
		ws.ActiveTabIdx = (ws.ActiveTabIdx + d + n) % n
	}
}

// GoToTab switches to the tab at 0-based index. No-op if out of range.
func (m *Manager) GoToTab(idx int) {
	if ws := m.ActiveWorkspace(); ws != nil && idx >= 0 && idx < len(ws.Tabs) {
		ws.ActiveTabIdx = idx
	}
}

// RenameTab renames the active tab.
func (m *Manager) RenameTab(name string) {
	if tab := m.ActiveTab(); tab != nil {
		tab.Name = name
	}
}

// CloseTab closes the active tab and all its panes.
// Returns true if nothing is left in the session.
func (m *Manager) CloseTab() bool {
	if tab := m.ActiveTab(); tab != nil {
		for _, p := range layout.CollectPanes(tab.Layout) {
			p.Close()
		}
	}
	return m.Reap()
}

// ActiveTab returns the currently active tab.
func (m *Manager) ActiveTab() *Tab {
	ws := m.ActiveWorkspace()
	if ws == nil || len(ws.Tabs) == 0 {
		return nil
	}
	return ws.Tabs[ws.ActiveTabIdx]
}

// ─── Pane ───────────────────────────────────────────────────────────────────

// SplitPane splits the active pane in the given direction and focuses the
// new pane, which starts in the active pane's working directory.
func (m *Manager) SplitPane(dir pane.SplitDirection) error {
	tab := m.ActiveTab()
	if tab == nil || tab.ActivePane == nil {
		return fmt.Errorf("no active pane")
	}
	node := layout.FindLeaf(tab.Layout, tab.ActivePane)
	if node == nil {
		return fmt.Errorf("active pane not found in layout")
	}
	// Start the new shell at the exact size it will get, so it doesn't
	// draw its first prompt at one size and immediately get resized.
	r, c, rows, cols := tab.ActivePane.Rect()
	_, b, _ := layout.Split(layout.Rect{Row: r, Col: c, Rows: rows, Cols: cols}, dir, 0.5)
	newP, err := m.newPane(b.Rows, b.Cols, tab.ActivePane.Cwd())
	if err != nil {
		return err
	}
	layout.SplitNode(node, dir, newP)
	tab.ActivePane = newP
	return nil
}

// ActivePane returns the currently focused pane.
func (m *Manager) ActivePane() *pane.Pane {
	if tab := m.ActiveTab(); tab != nil {
		return tab.ActivePane
	}
	return nil
}

// ActivePanes returns all panes in the current tab.
func (m *Manager) ActivePanes() []*pane.Pane {
	if tab := m.ActiveTab(); tab != nil {
		return layout.CollectPanes(tab.Layout)
	}
	return nil
}

// KillActivePane closes the active pane; its sibling takes over the space
// and the focus. Returns true if nothing is left in the session.
func (m *Manager) KillActivePane() bool {
	if p := m.ActivePane(); p != nil {
		p.Close()
	}
	return m.Reap()
}

// Reap removes every dead pane in every tab of every workspace, then any
// tab or workspace left empty. It is the single place panes leave the
// tree, whether closed by the user or because their shell exited.
// Returns true if nothing is left in the session.
func (m *Manager) Reap() bool {
	for wi := 0; wi < len(m.Workspaces); {
		ws := m.Workspaces[wi]
		for ti := 0; ti < len(ws.Tabs); {
			tab := ws.Tabs[ti]
			for _, p := range layout.CollectPanes(tab.Layout) {
				if !p.Dead() {
					continue
				}
				var heir *pane.Pane
				tab.Layout, heir = layout.RemovePane(tab.Layout, p)
				if tab.ActivePane == p {
					tab.ActivePane = heir
				}
			}
			if tab.Layout == nil {
				ws.Tabs = slices.Delete(ws.Tabs, ti, ti+1)
				ws.ActiveTabIdx = removedIdx(ws.ActiveTabIdx, ti, len(ws.Tabs))
				continue
			}
			ti++
		}
		if len(ws.Tabs) == 0 {
			m.Workspaces = slices.Delete(m.Workspaces, wi, wi+1)
			m.ActiveWorkspaceIdx = removedIdx(m.ActiveWorkspaceIdx, wi, len(m.Workspaces))
			continue
		}
		wi++
	}
	return len(m.Workspaces) == 0
}

// removedIdx returns the new active index after element removed was deleted
// from a list that now has n elements: focus moves to the previous element,
// like closing a browser tab.
func removedIdx(active, removed, n int) int {
	if active >= removed && active > 0 {
		active--
	}
	return min(active, max(n-1, 0))
}

func (m *Manager) NextPane() { m.cyclePane(1) }
func (m *Manager) PrevPane() { m.cyclePane(-1) }

func (m *Manager) cyclePane(d int) {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	panes := layout.CollectPanes(tab.Layout)
	if i := slices.Index(panes, tab.ActivePane); i >= 0 {
		n := len(panes)
		tab.ActivePane = panes[(i+d+n)%n]
	}
}

// SelectPaneInDirection focuses the nearest pane that lies entirely on the
// given side ("up", "down", "left", "right") of the active pane and shares
// at least one row (left/right) or column (up/down) with it.
func (m *Manager) SelectPaneInDirection(direction string) {
	tab := m.ActiveTab()
	if tab == nil || tab.ActivePane == nil {
		return
	}
	r, c, h, w := tab.ActivePane.Rect()
	var best *pane.Pane
	bestDist, bestOff := 1<<30, 1<<30
	for _, p := range layout.CollectPanes(tab.Layout) {
		pr, pc, ph, pw := p.Rect()
		rowsOverlap := pr < r+h && r < pr+ph
		colsOverlap := pc < c+w && c < pc+pw
		var dist, off int
		switch {
		case direction == "left" && pc+pw <= c && rowsOverlap:
			dist, off = c-(pc+pw), abs(pr-r)
		case direction == "right" && pc >= c+w && rowsOverlap:
			dist, off = pc-(c+w), abs(pr-r)
		case direction == "up" && pr+ph <= r && colsOverlap:
			dist, off = r-(pr+ph), abs(pc-c)
		case direction == "down" && pr >= r+h && colsOverlap:
			dist, off = pr-(r+h), abs(pc-c)
		default:
			continue
		}
		if dist < bestDist || (dist == bestDist && off < bestOff) {
			best, bestDist, bestOff = p, dist, off
		}
	}
	if best != nil {
		tab.ActivePane = best
	}
}

func abs(x int) int { return max(x, -x) }

// SwapPanes swaps the active pane with the next one in the layout. Focus
// follows the active pane to its new position.
func (m *Manager) SwapPanes() {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	panes := layout.CollectPanes(tab.Layout)
	i := slices.Index(panes, tab.ActivePane)
	if len(panes) <= 1 || i < 0 {
		return
	}
	a := layout.FindLeaf(tab.Layout, panes[i])
	b := layout.FindLeaf(tab.Layout, panes[(i+1)%len(panes)])
	a.Pane, b.Pane = b.Pane, a.Pane
}

// ResizeRatio grows (delta > 0) or shrinks the active pane by moving the
// border of its enclosing split.
func (m *Manager) ResizeRatio(delta float64) {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	parent, idx := layout.FindParent(tab.Layout, tab.ActivePane)
	if parent == nil {
		return
	}
	// Ratio is the first child's share, so growing the second child means
	// lowering it.
	if idx == 1 {
		delta = -delta
	}
	parent.Ratio = min(max(parent.Ratio+delta, 0.1), 0.9)
}

// EqualizeLayout resets all split ratios to 0.5.
func (m *Manager) EqualizeLayout() {
	if tab := m.ActiveTab(); tab != nil {
		layout.Equalize(tab.Layout)
	}
}

// Shutdown terminates every pane in every tab in every workspace.
func (m *Manager) Shutdown() {
	for _, ws := range m.Workspaces {
		for _, tab := range ws.Tabs {
			for _, p := range layout.CollectPanes(tab.Layout) {
				p.Close()
			}
		}
	}
}
