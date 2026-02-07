package session

import (
	"fmt"
	"sync"

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
	nextPaneID         int
	shell              string
	mu                 sync.Mutex
}

// NewManager creates a new session manager with the given default shell.
func NewManager(shell string) *Manager {
	return &Manager{
		Workspaces: []*Workspace{},
		shell:      shell,
	}
}

// Init creates the first workspace and tab. Must be called after construction.
func (m *Manager) Init(rows, cols uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addWorkspaceLocked("Main", rows, cols)
}

func (m *Manager) addWorkspaceLocked(name string, rows, cols uint16) error {
	ws := &Workspace{Name: name, Tabs: []*Tab{}}
	m.Workspaces = append(m.Workspaces, ws)
	m.ActiveWorkspaceIdx = len(m.Workspaces) - 1
	return m.createTabLocked("Shell", rows, cols)
}

func (m *Manager) createTabLocked(name string, rows, cols uint16) error {
	ws := m.Workspaces[m.ActiveWorkspaceIdx]

	m.nextPaneID++
	p, err := pane.New(m.nextPaneID, rows, cols, m.shell)
	if err != nil {
		return fmt.Errorf("failed to create pane: %w", err)
	}

	tab := &Tab{
		Name:       name,
		Layout:     layout.NewLeaf(p),
		ActivePane: p,
	}
	ws.Tabs = append(ws.Tabs, tab)
	ws.ActiveTabIdx = len(ws.Tabs) - 1
	return nil
}

// AddWorkspace creates a new workspace with one tab.
func (m *Manager) AddWorkspace(name string, rows, cols uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addWorkspaceLocked(name, rows, cols)
}

// CreateTab creates a new tab in the current workspace.
func (m *Manager) CreateTab(name string, rows, cols uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createTabLocked(name, rows, cols)
}

// SplitPane splits the active pane in the given direction.
func (m *Manager) SplitPane(dir pane.SplitDirection, rows, cols uint16) (*pane.Pane, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tab := m.activeTabLocked()
	if tab == nil {
		return nil, fmt.Errorf("no active tab")
	}

	node := layout.FindLeaf(tab.Layout, tab.ActivePane)
	if node == nil {
		return nil, fmt.Errorf("active pane not found in layout")
	}

	m.nextPaneID++
	var pRows, pCols uint16
	if dir == pane.SplitVertical {
		pRows = rows
		pCols = cols / 2
	} else {
		pRows = rows / 2
		pCols = cols
	}
	if pRows < 1 {
		pRows = 1
	}
	if pCols < 1 {
		pCols = 1
	}

	newP, err := pane.New(m.nextPaneID, pRows, pCols, m.shell)
	if err != nil {
		return nil, err
	}

	layout.SplitNode(node, dir, newP)
	tab.ActivePane = newP

	return newP, nil
}

// GetActivePane returns the currently focused pane.
func (m *Manager) GetActivePane() *pane.Pane {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return nil
	}
	return tab.ActivePane
}

// GetActiveTab returns the currently active tab.
func (m *Manager) GetActiveTab() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeTabLocked()
}

func (m *Manager) activeTabLocked() *Tab {
	if len(m.Workspaces) == 0 {
		return nil
	}
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) == 0 {
		return nil
	}
	return ws.Tabs[ws.ActiveTabIdx]
}

// GetCurrentWorkspace returns the currently active workspace.
func (m *Manager) GetCurrentWorkspace() *Workspace {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Workspaces) == 0 {
		return nil
	}
	return m.Workspaces[m.ActiveWorkspaceIdx]
}

// KillActivePane closes the active pane and adjusts the layout.
// Returns true if the last workspace is now empty (caller should quit).
func (m *Manager) KillActivePane() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	tab := m.activeTabLocked()
	if tab == nil {
		return false
	}

	activeP := tab.ActivePane
	if activeP == nil {
		return false
	}

	activeP.Close()

	tab.Layout = layout.RemovePane(tab.Layout, activeP)

	if tab.Layout == nil {
		// Tab has no more panes — remove the tab
		ws := m.Workspaces[m.ActiveWorkspaceIdx]
		ws.Tabs = append(ws.Tabs[:ws.ActiveTabIdx], ws.Tabs[ws.ActiveTabIdx+1:]...)
		if len(ws.Tabs) == 0 {
			return true // workspace empty — signal exit
		}
		if ws.ActiveTabIdx >= len(ws.Tabs) {
			ws.ActiveTabIdx = len(ws.Tabs) - 1
		}
		newTab := ws.Tabs[ws.ActiveTabIdx]
		panes := layout.CollectPanes(newTab.Layout)
		if len(panes) > 0 {
			newTab.ActivePane = panes[0]
		}
		return false
	}

	// Focus the first remaining pane
	panes := layout.CollectPanes(tab.Layout)
	if len(panes) > 0 {
		tab.ActivePane = panes[0]
	}

	return false
}

// NextPane cycles focus to the next pane in the current tab.
func (m *Manager) NextPane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return
	}
	panes := layout.CollectPanes(tab.Layout)
	if len(panes) <= 1 {
		return
	}
	for i, p := range panes {
		if p == tab.ActivePane {
			tab.ActivePane = panes[(i+1)%len(panes)]
			return
		}
	}
}

// PrevPane cycles focus to the previous pane in the current tab.
func (m *Manager) PrevPane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return
	}
	panes := layout.CollectPanes(tab.Layout)
	if len(panes) <= 1 {
		return
	}
	for i, p := range panes {
		if p == tab.ActivePane {
			idx := i - 1
			if idx < 0 {
				idx = len(panes) - 1
			}
			tab.ActivePane = panes[idx]
			return
		}
	}
}

// NextTab cycles to the next tab in the current workspace.
func (m *Manager) NextTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) <= 1 {
		return
	}
	ws.ActiveTabIdx = (ws.ActiveTabIdx + 1) % len(ws.Tabs)
}

// PrevTab cycles to the previous tab in the current workspace.
func (m *Manager) PrevTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) <= 1 {
		return
	}
	ws.ActiveTabIdx--
	if ws.ActiveTabIdx < 0 {
		ws.ActiveTabIdx = len(ws.Tabs) - 1
	}
}

// NextWorkspace cycles to the next workspace.
func (m *Manager) NextWorkspace() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Workspaces) <= 1 {
		return
	}
	m.ActiveWorkspaceIdx = (m.ActiveWorkspaceIdx + 1) % len(m.Workspaces)
}

// ApplyLayout recalculates positions and sizes for all panes in the active tab.
func (m *Manager) ApplyLayout(area layout.Rect) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return
	}
	layout.Apply(tab.Layout, area)
}

// GetAllPanesInActiveTab returns all panes in the current tab.
func (m *Manager) GetAllPanesInActiveTab() []*pane.Pane {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return nil
	}
	return layout.CollectPanes(tab.Layout)
}

// CleanupDeadPanes removes any dead panes from the active tab.
// Returns true if all panes/tabs are gone (caller should quit).
func (m *Manager) CleanupDeadPanes() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	tab := m.activeTabLocked()
	if tab == nil {
		return false
	}

	panes := layout.CollectPanes(tab.Layout)
	for _, p := range panes {
		if p.Dead() {
			tab.Layout = layout.RemovePane(tab.Layout, p)
			if tab.Layout == nil {
				ws := m.Workspaces[m.ActiveWorkspaceIdx]
				ws.Tabs = append(ws.Tabs[:ws.ActiveTabIdx], ws.Tabs[ws.ActiveTabIdx+1:]...)
				if len(ws.Tabs) == 0 {
					return true
				}
				if ws.ActiveTabIdx >= len(ws.Tabs) {
					ws.ActiveTabIdx = len(ws.Tabs) - 1
				}
				newTab := ws.Tabs[ws.ActiveTabIdx]
				remaining := layout.CollectPanes(newTab.Layout)
				if len(remaining) > 0 {
					newTab.ActivePane = remaining[0]
				}
				return false
			}
			remaining := layout.CollectPanes(tab.Layout)
			if len(remaining) > 0 {
				tab.ActivePane = remaining[0]
			}
			return false
		}
	}
	return false
}
