// Package session holds the workspace / tab / pane tree.
//
// Manager is not safe for concurrent use: app.App serializes every call
// behind its own mutex.
package session

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
)

// Tab holds a layout tree and an active pane pointer.
type Tab struct {
	Name       string
	CustomName bool
	Stacked    bool
	LayoutMode layout.Mode
	prevMode   layout.Mode
	Layout     *layout.Node
	ActivePane *pane.Pane
	Zoomed     *pane.Pane
}

// Workspace groups tabs under a named context.
type Workspace struct {
	Name         string
	Tabs         []*Tab
	ActiveTabIdx int
}

// Manager manages all workspaces, tabs, and panes.
type Manager struct {
	SessionName        string
	Workspaces         []*Workspace
	ActiveWorkspaceIdx int

	shell      string
	scrollback int
	onChange   func()
	nextPane   int
	nextWS     int
}

// NewManager creates a session manager whose panes run shell and keep
// scrollback lines of history; onChange is passed through to pane.New.
func NewManager(shell string, scrollback int, onChange func()) *Manager {
	return &Manager{SessionName: "slat", shell: shell, scrollback: scrollback, onChange: onChange}
}

// UpdateDefaults updates the shell and scrollback used for newly created panes.
func (m *Manager) UpdateDefaults(shell string, scrollback int) {
	m.shell = shell
	m.scrollback = scrollback
}

func (m *Manager) newPane(rows, cols int, dir string) (*pane.Pane, error) {
	m.nextPane++
	p, err := pane.New(m.nextPane, rows, cols, m.shell, dir, m.scrollback, m.onChange)
	if err != nil {
		return nil, fmt.Errorf("failed to start %s: %w", m.shell, err)
	}
	return p, nil
}

// Issued reports how many panes this session has ever created. Ids are
// never reused, so any id at or below it named a pane that is now gone.
func (m *Manager) Issued() int { return m.nextPane }

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
		LayoutMode: layout.ModeEvenVertical,
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

// RenameSession renames the session.
func (m *Manager) RenameSession(name string) {
	m.SessionName = name
}

var (
	sessionPrefixes = []string{
		"chaotic", "distilled", "arcane", "ethereal", "cursed", "blessed", "crimson", "hollow",
		"astral", "void", "feral", "gilded", "primal", "shattered", "runic", "spectral",
		"molten", "frozen", "ancient", "wicked", "radiant", "twilight", "phantom", "iron",
		"obsidian", "velvet", "fervent", "stormy", "silent", "wild", "vivid", "lucid",
		"umbra", "eldritch", "savage", "amber", "cobalt", "brittle", "mystic", "grim",
	}
	sessionSuffixes = []string{
		"essence", "monkey", "shard", "blade", "echo", "ember", "raven", "anvil",
		"orb", "whisper", "lantern", "spire", "prism", "sigil", "relic", "falcon",
		"viper", "tome", "chalice", "beacon", "cinder", "crown", "compass", "spark",
		"fox", "lynx", "wolf", "stag", "crane", "moth", "mantis", "serpent",
		"griffin", "phoenix", "drake", "golem", "wraith", "warden", "nomad", "herald",
	}
)

// SessionDictionaries returns the prefix and suffix word lists used for
// random session names.
func SessionDictionaries() (prefixes, suffixes []string) {
	return sessionPrefixes, sessionSuffixes
}

// RandomSessionName generates a <prefix>-<suffix> name, re-rolling if the
// candidate is already present in inUse.
func RandomSessionName(inUse map[string]bool) string {
	for attempt := 0; attempt < 64; attempt++ {
		p := sessionPrefixes[rand.IntN(len(sessionPrefixes))]
		s := sessionSuffixes[rand.IntN(len(sessionSuffixes))]
		name := p + "-" + s
		if !inUse[name] {
			return name
		}
	}
	base := sessionPrefixes[rand.IntN(len(sessionPrefixes))] + "-" + sessionSuffixes[rand.IntN(len(sessionSuffixes))]
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !inUse[candidate] {
			return candidate
		}
	}
}

// RenameTab renames the active tab.
func (m *Manager) RenameTab(name string) {
	if tab := m.ActiveTab(); tab != nil {
		tab.Name = name
		tab.CustomName = true
	}
}

// ToggleStack toggles stacked pane mode on the active tab.
func (m *Manager) ToggleStack() {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	tab.Zoomed = nil
	if tab.Stacked {
		prev := tab.prevMode
		if prev == "" || prev == layout.ModeStack {
			prev = layout.ModeEvenVertical
		}
		tab.Stacked = false
		tab.LayoutMode = prev
	} else {
		if tab.LayoutMode != layout.ModeStack {
			tab.prevMode = tab.LayoutMode
		}
		tab.Stacked = true
		tab.LayoutMode = layout.ModeStack
	}
}

// ApplyLayout rearranges the active tab's panes according to mode.
func (m *Manager) ApplyLayout(mode layout.Mode) {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	tab.Zoomed = nil
	if mode != layout.ModeStack {
		tab.prevMode = mode
	}
	tab.LayoutMode = mode
	if mode == layout.ModeStack {
		tab.Stacked = true
		return
	}
	tab.Stacked = false
	panes := layout.CollectPanes(tab.Layout)
	if len(panes) > 1 {
		tab.Layout = layout.Arrange(panes, mode)
	}
}

// FocusPane sets the active pane in the active tab to p.
func (m *Manager) FocusPane(p *pane.Pane) {
	if tab := m.ActiveTab(); tab != nil && p != nil {
		if tab.Zoomed != p {
			tab.Zoomed = nil
		}
		tab.ActivePane = p
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
	_, err := m.SplitAt(m.ActivePane(), dir, "")
	return err
}

// AddPane adds a new pane to the active tab and arranges all panes in the tab
// according to the tab's current layout mode.
func (m *Manager) AddPane(area layout.Rect) error {
	tab := m.ActiveTab()
	if tab == nil {
		return errors.New("no active tab")
	}
	newP, err := m.newPane(max(area.Rows, 1), max(area.Cols, 1), m.activeDir())
	if err != nil {
		return err
	}
	m.attachPaneToTab(tab, newP, area)
	return nil
}

func (m *Manager) attachPaneToTab(tab *Tab, p *pane.Pane, area layout.Rect) {
	panes := append(layout.CollectPanes(tab.Layout), p)
	mode := tab.LayoutMode
	if mode == "" {
		if tab.Stacked {
			mode = layout.ModeStack
		} else {
			mode = layout.ModeEvenVertical
		}
	}
	tab.LayoutMode = mode
	tab.Layout = layout.Arrange(panes, mode)
	tab.ActivePane = p
	tab.Zoomed = nil
	if !tab.Stacked {
		layout.Apply(tab.Layout, area)
	}
}

// BreakPane moves the active pane out of the current tab into a new tab in the
// active workspace and switches focus to it.
func (m *Manager) BreakPane() error {
	ws := m.ActiveWorkspace()
	tab := m.ActiveTab()
	p := m.ActivePane()
	if ws == nil || tab == nil || p == nil {
		return errors.New("no active pane")
	}
	if layout.CountPanes(tab.Layout) <= 1 {
		return errors.New("only one pane in tab")
	}
	var heir *pane.Pane
	tab.Layout, heir = layout.RemovePane(tab.Layout, p)
	tab.ActivePane = heir
	if tab.Zoomed == p || (tab.Zoomed != nil && tab.Layout != nil && tab.Layout.Pane != nil) {
		tab.Zoomed = nil
	}

	_, name, _ := p.Foreground()
	if name == "" {
		name = "shell"
	}
	ws.Tabs = append(ws.Tabs, &Tab{
		Name:       name,
		LayoutMode: layout.ModeEvenVertical,
		Layout:     layout.NewLeaf(p),
		ActivePane: p,
	})
	ws.ActiveTabIdx = len(ws.Tabs) - 1
	return nil
}

// JoinPane moves src from its current tab into the active tab, arranging panes
// according to the active tab's layout mode and removing any emptied source tab.
func (m *Manager) JoinPane(src *pane.Pane, area layout.Rect) error {
	return m.movePaneToTab(src, m.ActiveTab(), area)
}

// MovePaneToTab moves src from its current tab into the tab at (dstWSIdx, dstTabIdx),
// arranging panes according to the destination tab's layout mode, removing any
// emptied source tab/workspace, and focusing the moved pane in the destination tab.
func (m *Manager) MovePaneToTab(src *pane.Pane, dstWSIdx, dstTabIdx int, area layout.Rect) error {
	if dstWSIdx < 0 || dstWSIdx >= len(m.Workspaces) {
		return errors.New("workspace not found")
	}
	dstWS := m.Workspaces[dstWSIdx]
	if dstTabIdx < 0 || dstTabIdx >= len(dstWS.Tabs) {
		return errors.New("tab not found")
	}
	if err := m.movePaneToTab(src, dstWS.Tabs[dstTabIdx], area); err != nil {
		return err
	}
	// Resolve destination indices after removal in case an emptied source
	// tab or workspace shifted indices.
	m.Focus(src)
	return nil
}

func (m *Manager) movePaneToTab(src *pane.Pane, dstTab *Tab, area layout.Rect) error {
	if dstTab == nil || src == nil {
		return errors.New("no active tab")
	}
	srcWSIdx, srcTabIdx := -1, -1
	for wi, ws := range m.Workspaces {
		for ti, tab := range ws.Tabs {
			if slices.Contains(layout.CollectPanes(tab.Layout), src) {
				srcWSIdx, srcTabIdx = wi, ti
				break
			}
		}
		if srcWSIdx >= 0 {
			break
		}
	}
	if srcWSIdx < 0 {
		return errors.New("pane not found")
	}
	srcWS := m.Workspaces[srcWSIdx]
	srcTab := srcWS.Tabs[srcTabIdx]
	if srcTab == dstTab {
		return errors.New("pane is already in current tab")
	}

	var heir *pane.Pane
	srcTab.Layout, heir = layout.RemovePane(srcTab.Layout, src)
	if srcTab.ActivePane == src {
		srcTab.ActivePane = heir
	}
	if srcTab.Zoomed == src || (srcTab.Zoomed != nil && srcTab.Layout != nil && srcTab.Layout.Pane != nil) {
		srcTab.Zoomed = nil
	}
	if srcTab.Layout == nil {
		srcWS.Tabs = slices.Delete(srcWS.Tabs, srcTabIdx, srcTabIdx+1)
		srcWS.ActiveTabIdx = removedIdx(srcWS.ActiveTabIdx, srcTabIdx, len(srcWS.Tabs))
		if len(srcWS.Tabs) == 0 {
			m.Workspaces = slices.Delete(m.Workspaces, srcWSIdx, srcWSIdx+1)
			m.ActiveWorkspaceIdx = removedIdx(m.ActiveWorkspaceIdx, srcWSIdx, len(m.Workspaces))
		}
	}

	m.attachPaneToTab(dstTab, src, area)
	return nil
}

// SplitAt splits the given pane, focuses the new one and returns it. The
// new pane starts in dir, or in the split pane's own directory when dir
// is empty.
func (m *Manager) SplitAt(target *pane.Pane, d pane.SplitDirection, dir string) (*pane.Pane, error) {
	if target == nil {
		return nil, errors.New("no such pane")
	}
	tab := m.tabOf(target)
	if tab == nil {
		return nil, fmt.Errorf("pane %d is not in the session", target.ID)
	}
	node := layout.FindLeaf(tab.Layout, target)
	if node == nil {
		return nil, fmt.Errorf("pane %d not found in layout", target.ID)
	}
	if dir == "" {
		dir = target.Cwd()
	}
	// Start the new shell at the exact size it will get, so it doesn't
	// draw its first prompt at one size and immediately get resized.
	r, c, rows, cols := target.Rect()
	_, b, _ := layout.Split(layout.Rect{Row: r, Col: c, Rows: rows, Cols: cols}, d, 0.5)
	newP, err := m.newPane(b.Rows, b.Cols, dir)
	if err != nil {
		return nil, err
	}
	layout.SplitNode(node, d, newP)
	tab.ActivePane = newP
	tab.Zoomed = nil
	return newP, nil
}

// PaneRef says where a pane sits in the session.
type PaneRef struct {
	Pane      *pane.Pane
	Workspace string
	TabIndex  int // 1-based, as the status bar shows it
	TabName   string
	Active    bool // the focused pane of the focused tab and workspace
}

// AllPanes lists every pane in every tab of every workspace.
func (m *Manager) AllPanes() []PaneRef {
	active := m.ActivePane()
	var out []PaneRef
	for _, ws := range m.Workspaces {
		for ti, tab := range ws.Tabs {
			for _, p := range layout.CollectPanes(tab.Layout) {
				out = append(out, PaneRef{
					Pane: p, Workspace: ws.Name, TabIndex: ti + 1,
					TabName: tab.Name, Active: p == active,
				})
			}
		}
	}
	return out
}

// PaneByID returns the pane with that id, or nil.
func (m *Manager) PaneByID(id int) *pane.Pane {
	for _, r := range m.AllPanes() {
		if r.Pane.ID == id {
			return r.Pane
		}
	}
	return nil
}

// Focus makes p the active pane, switching workspace and tab to reach it.
func (m *Manager) Focus(p *pane.Pane) bool {
	for wi, ws := range m.Workspaces {
		for ti, tab := range ws.Tabs {
			if slices.Contains(layout.CollectPanes(tab.Layout), p) {
				if tab.Zoomed != p {
					tab.Zoomed = nil
				}
				m.ActiveWorkspaceIdx, ws.ActiveTabIdx, tab.ActivePane = wi, ti, p
				return true
			}
		}
	}
	return false
}

// tabOf finds the tab holding p.
func (m *Manager) tabOf(p *pane.Pane) *Tab {
	for _, ws := range m.Workspaces {
		for _, tab := range ws.Tabs {
			if slices.Contains(layout.CollectPanes(tab.Layout), p) {
				return tab
			}
		}
	}
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
				if tab.Zoomed == p {
					tab.Zoomed = nil
				}
			}
			if tab.Layout == nil {
				ws.Tabs = slices.Delete(ws.Tabs, ti, ti+1)
				ws.ActiveTabIdx = removedIdx(ws.ActiveTabIdx, ti, len(ws.Tabs))
				continue
			}
			if tab.Zoomed != nil && tab.Layout.Pane != nil {
				tab.Zoomed = nil
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
	tab.Zoomed = nil
	panes := layout.CollectPanes(tab.Layout)
	if i := slices.Index(panes, tab.ActivePane); i >= 0 {
		n := len(panes)
		tab.ActivePane = panes[(i+d+n)%n]
	}
}

// SelectPaneInDirection focuses the nearest pane that lies entirely on the
// given side ("up", "down", "left", "right") of the active pane.
func (m *Manager) SelectPaneInDirection(direction string) {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	tab.Zoomed = nil
	if p := m.paneInDirection(tab, direction); p != nil {
		tab.ActivePane = p
	}
}

// MovePaneInDirection swaps the active pane with its neighbour on that
// side, so a pane can be walked around the layout the way a tiling window
// manager moves a window. Focus stays on the moved pane. It reports the
// two panes it exchanged, and false when there is no neighbour that way.
func (m *Manager) MovePaneInDirection(direction string) (*pane.Pane, *pane.Pane, bool) {
	tab := m.ActiveTab()
	if tab == nil || tab.ActivePane == nil {
		return nil, nil, false
	}
	neighbour := m.paneInDirection(tab, direction)
	if neighbour == nil {
		return nil, nil, false
	}
	here := layout.FindLeaf(tab.Layout, tab.ActivePane)
	there := layout.FindLeaf(tab.Layout, neighbour)
	if here == nil || there == nil {
		return nil, nil, false
	}
	here.Pane, there.Pane = there.Pane, here.Pane
	return tab.ActivePane, neighbour, true
}

// paneInDirection is the nearest pane entirely on that side of the active
// one, sharing at least one row (left/right) or column (up/down) with it.
func (m *Manager) paneInDirection(tab *Tab, direction string) *pane.Pane {
	if tab.ActivePane == nil {
		return nil
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
	return best
}

func abs(x int) int { return max(x, -x) }

// SwapPanes swaps the active pane with the next one in the layout. Focus
// follows the active pane to its new position.
func (m *Manager) SwapPanes() {
	tab := m.ActiveTab()
	if tab == nil {
		return
	}
	tab.Zoomed = nil
	panes := layout.CollectPanes(tab.Layout)
	i := slices.Index(panes, tab.ActivePane)
	if len(panes) <= 1 || i < 0 {
		return
	}
	tab.Zoomed = nil
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
	tab.Zoomed = nil
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
		tab.Zoomed = nil
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
