package app

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/ui"
)

// RemoteTreeNode is a serializable tree entry for cross-session global tree.
type RemoteTreeNode struct {
	Label      string `json:"label"`
	SearchText string `json:"search_text"`
	Workspace  int    `json:"workspace"`
	Tab        int    `json:"tab"`
	PaneID     int    `json:"pane_id,omitempty"`
	Socket     string `json:"socket,omitempty"`
}

type treeNode struct {
	label      string
	searchText string
	wsIdx      int
	tabIdx     int        // -1 for workspace node
	pane       *pane.Pane // nil for workspace/tab nodes
	paneID     int        // used for remote pane focus
	socket     string     // non-empty for cross-session picker items
	layoutMode layout.Mode
	active     bool
}

type treeState struct {
	title    string
	query    []byte
	all      []treeNode
	filtered []treeNode
	cursor   int
	offset   int
	onSelect func(treeNode)
}

func (a *App) openTree() {
	all := a.buildTree()
	ts := &treeState{title: "session tree", all: all}
	ts.refilter()
	// Start cursor on the currently active pane if present.
	for i, n := range ts.filtered {
		if n.active && n.pane != nil {
			ts.cursor = i
			break
		}
	}
	a.tree = ts
}

func (a *App) openLayoutPicker() {
	var cur layout.Mode
	if tab := a.manager.ActiveTab(); tab != nil {
		cur = tab.LayoutMode
		if cur == "" {
			if tab.Stacked {
				cur = layout.ModeStack
			} else {
				cur = layout.ModeEvenVertical
			}
		}
	}
	modes := layout.AllModes()
	all := make([]treeNode, 0, len(modes))
	for _, m := range modes {
		all = append(all, treeNode{
			label:      fmt.Sprintf("%-16s  %s", string(m.Mode), m.Description),
			searchText: fmt.Sprintf("%s %s", string(m.Mode), m.Description),
			layoutMode: m.Mode,
			active:     m.Mode == cur,
		})
	}
	ts := &treeState{
		title: "layouts",
		all:   all,
		onSelect: func(n treeNode) {
			a.manager.ApplyLayout(n.layoutMode)
		},
	}
	ts.refilter()
	for i, n := range ts.filtered {
		if n.active {
			ts.cursor = i
			break
		}
	}
	a.tree = ts
}

func (a *App) openGlobalTreePicker() {
	all := a.buildTree()
	if a.listGlobalTree != nil {
		for _, rn := range a.listGlobalTree() {
			all = append(all, treeNode{
				label:      rn.Label,
				searchText: rn.SearchText,
				wsIdx:      rn.Workspace,
				tabIdx:     rn.Tab,
				paneID:     rn.PaneID,
				socket:     rn.Socket,
			})
		}
	}
	ts := &treeState{title: "all sessions", all: all}
	ts.refilter()
	for i, n := range ts.filtered {
		if n.active && n.pane != nil {
			ts.cursor = i
			break
		}
	}
	a.tree = ts
}

// ExportTree returns a serializable tree of the current session for `tree` RPC.
func (a *App) ExportTree() []RemoteTreeNode {
	a.mu.Lock()
	defer a.mu.Unlock()
	nodes := a.buildTree()
	out := make([]RemoteTreeNode, len(nodes))
	for i, n := range nodes {
		pid := 0
		if n.pane != nil {
			pid = n.pane.ID
		}
		out[i] = RemoteTreeNode{
			Label:      n.label,
			SearchText: n.searchText,
			Workspace:  n.wsIdx,
			Tab:        n.tabIdx,
			PaneID:     pid,
		}
	}
	return out
}

// FocusTarget focuses a workspace, tab, and optional pane ID in this session.
func (a *App) FocusTarget(wsIdx, tabIdx, paneID int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.markDirty()
	if wsIdx >= 0 && wsIdx < len(a.manager.Workspaces) {
		a.manager.ActiveWorkspaceIdx = wsIdx
	}
	ws := a.manager.ActiveWorkspace()
	if ws != nil && tabIdx >= 0 && tabIdx < len(ws.Tabs) {
		ws.ActiveTabIdx = tabIdx
		if paneID > 0 {
			for _, p := range layout.CollectPanes(ws.Tabs[tabIdx].Layout) {
				if p.ID == paneID {
					a.manager.FocusPane(p)
					break
				}
			}
		}
	}
}

func (a *App) allSessionsLocked() (SessionSummary, []SessionSummary) {
	cur := a.sessionInfoLocked()
	sessions := []SessionSummary{cur}
	if a.listSessions != nil {
		for _, s := range a.listSessions() {
			if s.Dir == cur.Dir && s.Name == cur.Name {
				continue
			}
			sessions = append(sessions, s)
		}
	}
	return cur, sessions
}

func (a *App) switchSession(delta int) {
	cur, sessions := a.allSessionsLocked()
	if len(sessions) <= 1 {
		a.notify("no other sessions")
		return
	}
	// Sort deterministically so next/prev cycle identically from every session.
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Name != sessions[j].Name {
			return sessions[i].Name < sessions[j].Name
		}
		if sessions[i].Dir != sessions[j].Dir {
			return sessions[i].Dir < sessions[j].Dir
		}
		return sessions[i].Socket < sessions[j].Socket
	})
	idx := 0
	for i, s := range sessions {
		if s.Dir == cur.Dir && s.Name == cur.Name {
			idx = i
			break
		}
	}
	next := (idx + delta + len(sessions)) % len(sessions)
	a.requestSwitch(sessions[next].Socket)
}

func (a *App) openSessionPicker() {
	_, sessions := a.allSessionsLocked()
	var all []treeNode
	for i, s := range sessions {
		isCur := i == 0
		state := "detached"
		if s.Attached {
			state = "attached"
		}
		all = append(all, treeNode{
			label:      fmt.Sprintf("%s  (%d tabs, %d panes, %s)  %s", s.Name, s.Tabs, s.Panes, state, s.Dir),
			searchText: fmt.Sprintf("%s %s %s", s.Name, s.Dir, state),
			socket:     s.Socket,
			active:     isCur,
		})
	}
	ts := &treeState{title: "sessions", all: all}
	ts.refilter()
	for i, n := range ts.filtered {
		if n.active {
			ts.cursor = i
			break
		}
	}
	a.tree = ts
}

func (a *App) buildTree() []treeNode {
	var out []treeNode
	sess := a.manager.SessionName
	if sess == "" {
		sess = "slat"
	}
	activeWS := a.manager.ActiveWorkspaceIdx
	for wi, ws := range a.manager.Workspaces {
		wsActive := wi == activeWS
		for ti, tab := range ws.Tabs {
			tabActive := wsActive && ti == ws.ActiveTabIdx
			panes := layout.CollectPanes(tab.Layout)
			for pi, p := range panes {
				_, fg, _ := p.Foreground()
				if fg == "" {
					fg = "shell"
				}
				paneActive := tabActive && p == tab.ActivePane
				out = append(out, treeNode{
					label:      fmt.Sprintf("%s / %s · %d:%s · pane %d (%s)", sess, ws.Name, ti+1, tab.Name, pi+1, fg),
					searchText: fmt.Sprintf("%s %s %d:%s %s pane %d %s", sess, ws.Name, ti+1, tab.Name, tab.Name, pi+1, fg),
					wsIdx:      wi,
					tabIdx:     ti,
					pane:       p,
					active:     paneActive,
				})
			}
		}
	}
	return out
}

func (ts *treeState) refilter() {
	q := strings.TrimSpace(string(ts.query))
	if q == "" {
		ts.filtered = append(ts.filtered[:0], ts.all...)
		ts.clampCursor()
		return
	}
	type scored struct {
		node  treeNode
		score int
		order int
	}
	var matches []scored
	for idx, n := range ts.all {
		if sc, ok := fuzzyMatch(q, n.searchText); ok {
			matches = append(matches, scored{node: n, score: sc, order: idx})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].order < matches[j].order
	})
	ts.filtered = ts.filtered[:0]
	for _, m := range matches {
		ts.filtered = append(ts.filtered, m.node)
	}
	ts.cursor = 0
	ts.offset = 0
}

func (ts *treeState) clampCursor() {
	if len(ts.filtered) == 0 {
		ts.cursor, ts.offset = 0, 0
		return
	}
	if ts.cursor < 0 {
		ts.cursor = 0
	}
	if ts.cursor >= len(ts.filtered) {
		ts.cursor = len(ts.filtered) - 1
	}
}

func (ts *treeState) move(delta, listH int) {
	if len(ts.filtered) == 0 {
		return
	}
	ts.cursor += delta
	ts.clampCursor()
	if listH <= 0 {
		listH = 1
	}
	if ts.cursor < ts.offset {
		ts.offset = ts.cursor
	}
	if ts.cursor >= ts.offset+listH {
		ts.offset = ts.cursor - listH + 1
	}
}

func (a *App) treeListHeight() int {
	boxH := min(max(a.rows-4, 8), 20)
	if boxH > a.rows {
		boxH = a.rows
	}
	return max(boxH-4, 1)
}

func (a *App) jumpToTreeNode(n treeNode) {
	if n.socket != "" {
		if !n.active {
			if a.focusRemote != nil && (n.wsIdx >= 0 || n.tabIdx >= 0 || n.paneID > 0) {
				a.focusRemote(n.socket, n.wsIdx, n.tabIdx, n.paneID)
			}
			a.requestSwitch(n.socket)
		}
		return
	}
	if n.wsIdx < 0 || n.wsIdx >= len(a.manager.Workspaces) {
		return
	}
	a.manager.ActiveWorkspaceIdx = n.wsIdx
	ws := a.manager.ActiveWorkspace()
	if ws != nil && n.tabIdx >= 0 && n.tabIdx < len(ws.Tabs) {
		ws.ActiveTabIdx = n.tabIdx
		if n.pane != nil {
			a.manager.FocusPane(n.pane)
		}
	}
}

// treeKey handles input while the fuzzy tree picker is open.
func (a *App) treeKey(buf []byte, i int) int {
	ts := a.tree
	listH := a.treeListHeight()
	b := buf[i]
	switch b {
	case 0x1b: // Esc or arrow/page sequence
		if i+3 < len(buf) && buf[i+1] == '[' && buf[i+3] == '~' {
			page := max(listH-1, 1)
			switch buf[i+2] {
			case '5': // PageUp
				ts.move(-page, listH)
				return i + 3
			case '6': // PageDown
				ts.move(page, listH)
				return i + 3
			}
		}
		if i+2 < len(buf) && (buf[i+1] == '[' || buf[i+1] == 'O') {
			switch buf[i+2] {
			case 'A':
				ts.move(-1, listH)
				return i + 2
			case 'B':
				ts.move(1, listH)
				return i + 2
			}
			return len(buf) - 1 // drop other escape sequence
		}
		a.tree = nil
		return len(buf) - 1
	case 0x03, 0x07: // Ctrl-C, Ctrl-G
		a.tree = nil
	case '\r':
		if len(ts.filtered) > 0 && ts.cursor >= 0 && ts.cursor < len(ts.filtered) {
			sel := ts.filtered[ts.cursor]
			if ts.onSelect != nil {
				ts.onSelect(sel)
			} else {
				a.jumpToTreeNode(sel)
			}
		}
		a.tree = nil
	case 0x10, 0x0b: // Ctrl-P, Ctrl-K
		ts.move(-1, listH)
	case 0x0e, 0x0a: // Ctrl-N, Ctrl-J
		ts.move(1, listH)
	case 0x15: // Ctrl-U
		ts.query = ts.query[:0]
		ts.refilter()
	case 0x7f, 0x08: // Backspace
		if len(ts.query) > 0 {
			_, size := utf8.DecodeLastRune(ts.query)
			ts.query = ts.query[:len(ts.query)-size]
			ts.refilter()
		}
	default:
		if b >= 0x20 && len(ts.query) < 128 {
			ts.query = append(ts.query, b)
			ts.refilter()
		}
	}
	return i
}

func (a *App) drawTree(frame *ui.Frame) (int, int) {
	ts := a.tree
	listH := a.treeListHeight()
	if ts.cursor < ts.offset {
		ts.offset = ts.cursor
	}
	if ts.cursor >= ts.offset+listH {
		ts.offset = ts.cursor - listH + 1
	}
	rows := make([]ui.TreeRow, len(ts.filtered))
	for i, n := range ts.filtered {
		rows[i] = ui.TreeRow{
			Label:    n.label,
			Selected: i == ts.cursor,
			Active:   n.active,
		}
	}
	title := ts.title
	if title == "" {
		title = "session tree"
	}
	return ui.DrawTreePicker(frame, title, string(ts.query), rows, ts.offset, len(ts.all))
}

// ─── FuzzyMatchV2 (fzf / go-fuzzyfinder Smith-Waterman DP) ──────────────────

const (
	fzScoreMatch        = 16
	fzScoreGapStart     = -3
	fzScoreGapExtension = -1
	fzBonusBoundary     = 8
	fzBonusCamel123     = 7
	fzBonusConsecutive  = 4
	fzFirstCharMult     = 2
)

// fuzzyMatch matches all space-separated tokens in query against text using
// smart-case and fzf's FuzzyMatchV2 Smith-Waterman DP scoring.
func fuzzyMatch(query, text string) (int, bool) {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return 0, true
	}
	tRunes := []rune(text)
	total := 0
	for _, tok := range tokens {
		sc, ok := fuzzyMatchToken([]rune(tok), tRunes)
		if !ok {
			return 0, false
		}
		total += sc
	}
	return total, true
}

func fuzzyMatchToken(pat, text []rune) (int, bool) {
	m, n := len(pat), len(text)
	if m == 0 {
		return 0, true
	}
	if m > n {
		return 0, false
	}

	caseSensitive := false
	for _, r := range pat {
		if unicode.IsUpper(r) {
			caseSensitive = true
			break
		}
	}

	// Quick subsequence pre-check + boundary bonus precompute.
	pi := 0
	bonus := make([]int, n)
	prevClass := charNonWord
	for j, r := range text {
		class := classifyRune(r)
		bonus[j] = calcBonus(prevClass, class)
		prevClass = class

		tr := r
		if !caseSensitive {
			tr = unicode.ToLower(tr)
		}
		if pi < m && tr == pat[pi] {
			pi++
		}
	}
	if pi < m {
		return 0, false
	}

	// Smith-Waterman DP: H[j] = best score ending at text[j] for pat[0..i],
	// C[j] = consecutive match run length ending at text[j].
	H := make([]int, n)
	C := make([]int, n)
	const negInf = -1 << 29

	for i := 0; i < m; i++ {
		pr := pat[i]
		prevRowH := negInf
		prevRowC := 0
		prevBest := negInf
		inGap := false

		for j := 0; j < n; j++ {
			curH := H[j]
			curC := C[j]
			tr := text[j]
			if !caseSensitive {
				tr = unicode.ToLower(tr)
			}

			var matchScore int
			var runLen int
			if tr == pr && (i == 0 || prevRowH > negInf/2) {
				b := bonus[j]
				if i == 0 {
					b *= fzFirstCharMult
					matchScore = fzScoreMatch + b
					runLen = 1
				} else {
					diag := prevRowH + fzScoreMatch
					if prevRowC > 0 {
						consec := max(fzBonusConsecutive, b)
						if bonus[j-prevRowC] > consec {
							consec = bonus[j-prevRowC]
						}
						diag += consec
						runLen = prevRowC + 1
					} else {
						diag += b
						runLen = 1
					}
					matchScore = diag
				}
			} else {
				matchScore = negInf
			}

			var gapScore int
			if prevBest > negInf/2 {
				if inGap {
					gapScore = prevBest + fzScoreGapExtension
				} else {
					gapScore = prevBest + fzScoreGapStart
				}
			} else {
				gapScore = negInf
			}

			if matchScore >= gapScore {
				H[j] = matchScore
				C[j] = runLen
				inGap = false
			} else {
				H[j] = gapScore
				C[j] = 0
				inGap = true
			}
			if H[j] > prevBest {
				prevBest = H[j]
			}
			prevRowH = curH
			prevRowC = curC
		}
	}

	best := negInf
	for j := 0; j < n; j++ {
		if H[j] > best {
			best = H[j]
		}
	}
	return best, best > negInf/2
}

type charClass uint8

const (
	charNonWord charClass = iota
	charLower
	charUpper
	charNumber
)

func classifyRune(r rune) charClass {
	switch {
	case unicode.IsLower(r):
		return charLower
	case unicode.IsUpper(r):
		return charUpper
	case unicode.IsDigit(r):
		return charNumber
	default:
		return charNonWord
	}
}

func calcBonus(prev, cur charClass) int {
	if cur == charNonWord {
		return 0
	}
	if prev == charNonWord {
		return fzBonusBoundary
	}
	if (prev == charLower && cur == charUpper) || (prev != charNumber && cur == charNumber) {
		return fzBonusCamel123
	}
	return 0
}
