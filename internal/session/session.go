package session

import (
	"fmt"
	"sync"

	"github.com/noturbob/slat/internal/pane"
)

type Tab struct {
	Name          string
	Panes         []*pane.Pane
	ActivePaneIdx int
}

type Workspace struct {
	Name         string
	Tabs         []*Tab
	ActiveTabIdx int
}

type Manager struct {
	Workspaces         []*Workspace
	ActiveWorkspaceIdx int
	nextPaneID         int
	mu                 sync.Mutex
}

func NewManager() *Manager {
	m := &Manager{Workspaces: []*Workspace{}}
	m.AddWorkspace("Main")
	return m
}

func (m *Manager) AddWorkspace(name string) {
	ws := &Workspace{Name: name, Tabs: []*Tab{}}
	m.Workspaces = append(m.Workspaces, ws)
	m.CreateTab("Shell")
}

func (m *Manager) CreateTab(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	
	newTab := &Tab{Name: name, Panes: []*pane.Pane{}}
	ws.Tabs = append(ws.Tabs, newTab)
	ws.ActiveTabIdx = len(ws.Tabs) - 1
	
	// Create first pane immediately
	m.createPaneInternal(ws, newTab)
}

func (m *Manager) createPaneInternal(ws *Workspace, t *Tab) (*pane.Pane, error) {
	m.nextPaneID++
	p, err := pane.New(m.nextPaneID, 24, 80)
	if err != nil { return nil, err }
	t.Panes = append(t.Panes, p)
	t.ActivePaneIdx = len(t.Panes) - 1
	return p, nil
}

func (m *Manager) CreatePane() (*pane.Pane, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) == 0 { return nil, fmt.Errorf("no tabs") }
	t := ws.Tabs[ws.ActiveTabIdx]
	return m.createPaneInternal(ws, t)
}

func (m *Manager) GetActivePane() *pane.Pane {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Workspaces) == 0 { return nil }
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) == 0 { return nil }
	t := ws.Tabs[ws.ActiveTabIdx]
	if len(t.Panes) == 0 { return nil }
	if t.ActivePaneIdx >= len(t.Panes) { t.ActivePaneIdx = len(t.Panes) - 1 }
	return t.Panes[t.ActivePaneIdx]
}

func (m *Manager) GetCurrentWorkspace() *Workspace {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Workspaces) == 0 { return nil }
	return m.Workspaces[m.ActiveWorkspaceIdx]
}

func (m *Manager) KillActivePane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	t := ws.Tabs[ws.ActiveTabIdx]
	if len(t.Panes) == 0 { return }
	
	active := t.Panes[t.ActivePaneIdx]
	active.Close()
	
	// Remove pane
	t.Panes = append(t.Panes[:t.ActivePaneIdx], t.Panes[t.ActivePaneIdx+1:]...)
	
	// Adjust focus
	if len(t.Panes) > 0 {
		if t.ActivePaneIdx >= len(t.Panes) { t.ActivePaneIdx = len(t.Panes) - 1 }
		if t.ActivePaneIdx < 0 { t.ActivePaneIdx = 0 }
	}
}

func (m *Manager) NextPane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	t := ws.Tabs[ws.ActiveTabIdx]
	if len(t.Panes) <= 1 { return }
	t.ActivePaneIdx = (t.ActivePaneIdx + 1) % len(t.Panes)
}

// --- ADDED MISSING FUNCTION ---
func (m *Manager) PrevPane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	t := ws.Tabs[ws.ActiveTabIdx]
	if len(t.Panes) <= 1 { return }
	t.ActivePaneIdx--
	if t.ActivePaneIdx < 0 { t.ActivePaneIdx = len(t.Panes) - 1 }
}

func (m *Manager) NextTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) <= 1 { return }
	ws.ActiveTabIdx = (ws.ActiveTabIdx + 1) % len(ws.Tabs)
}

func (m *Manager) PrevTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	ws := m.Workspaces[m.ActiveWorkspaceIdx]
	if len(ws.Tabs) <= 1 { return }
	ws.ActiveTabIdx--
	if ws.ActiveTabIdx < 0 { ws.ActiveTabIdx = len(ws.Tabs) - 1 }
}