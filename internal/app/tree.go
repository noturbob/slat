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
	case 0x17: // Ctrl-W
		ts.query = deleteLastWord(ts.query)
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

// deleteLastWord removes trailing whitespace and the preceding non-whitespace
// word from b (standard Ctrl-W readline behavior).
func deleteLastWord(b []byte) []byte {
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if !unicode.IsSpace(r) {
			break
		}
		b = b[:len(b)-size]
	}
	for len(b) > 0 {
		r, size := utf8.DecodeLastRune(b)
		if unicode.IsSpace(r) {
			break
		}
		b = b[:len(b)-size]
	}
	return b
}

// ─── Word-Prefix & fzf Extended-Search Filter ───────────────────────────────

type filterMode uint8

const (
	filterWordPrefix filterMode = iota
	filterExact
)

type filterTerm struct {
	pat           []rune
	mode          filterMode
	anchorStart   bool
	anchorEnd     bool
	negate        bool
	caseSensitive bool
}

func (t *filterTerm) eval(text []rune) (int, bool) {
	return evalFilterTerm(*t, text)
}

type filterNode interface {
	eval(text []rune) (int, bool)
}

type filterNotNode struct{ child filterNode }

func (n filterNotNode) eval(text []rune) (int, bool) {
	if _, ok := n.child.eval(text); ok {
		return 0, false
	}
	return 0, true
}

type filterAndNode struct{ children []filterNode }

func (n filterAndNode) eval(text []rune) (int, bool) {
	total := 0
	for _, c := range n.children {
		sc, ok := c.eval(text)
		if !ok {
			return 0, false
		}
		total += sc
	}
	return total, true
}

type filterOrNode struct{ children []filterNode }

func (n filterOrNode) eval(text []rune) (int, bool) {
	matched := false
	best := 0
	for _, c := range n.children {
		sc, ok := c.eval(text)
		if !ok {
			continue
		}
		if !matched || sc > best {
			best = sc
		}
		matched = true
	}
	return best, matched
}

type queryTokenKind uint8

const (
	tokTerm queryTokenKind = iota
	tokLParen
	tokRParen
	tokOr
	tokNot
)

type queryToken struct {
	kind queryTokenKind
	raw  string
}

func tokenizeFilterQuery(query string) []queryToken {
	var tokens []queryToken
	i := 0
	for i < len(query) {
		r, size := utf8.DecodeRuneInString(query[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		switch r {
		case '(':
			tokens = append(tokens, queryToken{kind: tokLParen})
			i += size
		case ')':
			tokens = append(tokens, queryToken{kind: tokRParen})
			i += size
		case '|':
			tokens = append(tokens, queryToken{kind: tokOr})
			i += size
		case '!':
			// Treat '!' as a prefix negation operator when followed by '(' (with optional spaces).
			j := i + size
			for j < len(query) {
				nr, ns := utf8.DecodeRuneInString(query[j:])
				if !unicode.IsSpace(nr) {
					if nr == '(' {
						tokens = append(tokens, queryToken{kind: tokNot})
						i += size
						goto nextTok
					}
					break
				}
				j += ns
			}
			fallthrough
		default:
			start := i
			for i < len(query) {
				tr, ts := utf8.DecodeRuneInString(query[i:])
				if tr == '\\' && i+ts < len(query) {
					_, ns := utf8.DecodeRuneInString(query[i+ts:])
					i += ts + ns
					continue
				}
				if unicode.IsSpace(tr) || tr == '(' || tr == ')' || tr == '|' {
					break
				}
				i += ts
			}
			tokens = append(tokens, queryToken{kind: tokTerm, raw: query[start:i]})
		}
	nextTok:
	}
	return tokens
}

type filterParser struct {
	tokens []queryToken
	pos    int
}

func (p *filterParser) peek() (queryToken, bool) {
	if p.pos >= len(p.tokens) {
		return queryToken{}, false
	}
	return p.tokens[p.pos], true
}

func (p *filterParser) parseAnd() filterNode {
	var children []filterNode
	for {
		tok, ok := p.peek()
		if !ok || tok.kind == tokRParen {
			break
		}
		if tok.kind == tokOr {
			p.pos++
			continue
		}
		if node := p.parseOr(); node != nil {
			children = append(children, node)
		}
	}
	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	default:
		return filterAndNode{children: children}
	}
}

func (p *filterParser) parseOr() filterNode {
	first := p.parseUnary()
	var children []filterNode
	if first != nil {
		children = append(children, first)
	}
	for {
		tok, ok := p.peek()
		if !ok || tok.kind != tokOr {
			break
		}
		p.pos++
		if next := p.parseUnary(); next != nil {
			children = append(children, next)
		}
	}
	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	default:
		return filterOrNode{children: children}
	}
}

func (p *filterParser) parseUnary() filterNode {
	for {
		tok, ok := p.peek()
		if !ok || tok.kind == tokRParen {
			return nil
		}
		if tok.kind == tokOr {
			p.pos++
			continue
		}
		if tok.kind == tokNot {
			p.pos++
			child := p.parseUnary()
			if child == nil {
				return nil
			}
			return filterNotNode{child: child}
		}
		if tok.kind == tokLParen {
			p.pos++
			inner := p.parseAnd()
			if next, ok := p.peek(); ok && next.kind == tokRParen {
				p.pos++
			}
			return inner
		}
		p.pos++
		term, ok := parseFilterTerm(tok.raw)
		if !ok {
			continue
		}
		return &term
	}
}

func parseFilterExpr(query string) filterNode {
	tokens := tokenizeFilterQuery(query)
	if len(tokens) == 0 {
		return nil
	}
	p := &filterParser{tokens: tokens}
	var parts []filterNode
	for p.pos < len(p.tokens) {
		if node := p.parseAnd(); node != nil {
			parts = append(parts, node)
		}
		// Skip any unmatched ')' at the top level.
		if tok, ok := p.peek(); ok && tok.kind == tokRParen {
			p.pos++
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	default:
		return filterAndNode{children: parts}
	}
}

// fuzzyMatch matches query against text using word-prefix matching by default,
// space-separated AND clauses, | OR alternatives, parenthesis grouping,
// smart-case, and fzf extended-search modifiers (^prefix, suffix$, 'exact, !exclude).
func fuzzyMatch(query, text string) (int, bool) {
	expr := parseFilterExpr(query)
	if expr == nil {
		return 0, true
	}
	return expr.eval([]rune(text))
}

func parseFilterTerm(tok string) (filterTerm, bool) {
	var term filterTerm
	if strings.HasPrefix(tok, "!") {
		term.negate = true
		tok = tok[1:]
	}
	if strings.HasPrefix(tok, "'") {
		term.mode = filterExact
		tok = tok[1:]
	} else {
		if strings.HasPrefix(tok, "^") {
			term.anchorStart = true
			tok = tok[1:]
		}
		if strings.HasSuffix(tok, "$") && len(tok) > 0 && !isEscapedEnd(tok, len(tok)-1) {
			term.anchorEnd = true
			tok = tok[:len(tok)-1]
		}
	}
	pat := unescapeFilterPat(tok)
	if len(pat) == 0 {
		return filterTerm{}, false
	}
	term.pat = pat
	for _, r := range term.pat {
		if unicode.IsUpper(r) {
			term.caseSensitive = true
			break
		}
	}
	return term, true
}

// isEscapedEnd reports whether the byte at idx in s is preceded by an odd number of backslashes.
func isEscapedEnd(s string, idx int) bool {
	bs := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		bs++
	}
	return bs%2 == 1
}

func unescapeFilterPat(s string) []rune {
	var out []rune
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == '\\' {
			if i+size >= len(s) {
				break
			}
			nr, ns := utf8.DecodeRuneInString(s[i+size:])
			out = append(out, nr)
			i += size + ns
			continue
		}
		out = append(out, r)
		i += size
	}
	return out
}

func evalFilterTerm(term filterTerm, text []rune) (int, bool) {
	sc, matched := matchFilterTerm(term, text)
	if term.negate {
		if matched {
			return 0, false
		}
		return 0, true
	}
	return sc, matched
}

func matchFilterTerm(term filterTerm, text []rune) (int, bool) {
	m, n := len(term.pat), len(text)
	if m == 0 {
		return 0, true
	}
	if m > n {
		return 0, false
	}

	if term.anchorStart && term.anchorEnd {
		if n != m {
			return 0, false
		}
		return scoreMatchAt(term.pat, text, 0, term.caseSensitive)
	}
	if term.anchorStart {
		return scoreMatchAt(term.pat, text, 0, term.caseSensitive)
	}
	if term.anchorEnd {
		return scoreMatchAt(term.pat, text, n-m, term.caseSensitive)
	}

	best := 0
	found := false
	limit := n - m
	for i := 0; i <= limit; i++ {
		if term.mode == filterWordPrefix && !isWordBoundary(text, i) {
			continue
		}
		if sc, ok := scoreMatchAt(term.pat, text, i, term.caseSensitive); ok {
			if !found || sc > best {
				best = sc
				found = true
			}
		}
	}
	return best, found
}

func scoreMatchAt(pat, text []rune, start int, caseSensitive bool) (int, bool) {
	if start < 0 || start+len(pat) > len(text) {
		return 0, false
	}
	exactCase := 0
	for i, pr := range pat {
		tr := text[start+i]
		if tr == pr {
			exactCase++
		}
		if !caseSensitive {
			tr = unicode.ToLower(tr)
		}
		if tr != pr {
			return 0, false
		}
	}

	score := len(pat)*16 + exactCase*2 - start
	if start == 0 {
		score += 32
	}
	atStart := isWordBoundary(text, start)
	if atStart {
		score += 16
	}
	end := start + len(pat)
	atEnd := end == len(text) || !isWordRune(text[end]) || (end > 0 && unicode.IsLower(text[end-1]) && unicode.IsUpper(text[end]))
	if atEnd {
		score += 16
	}
	if atStart && atEnd {
		// Full-word matches always outrank partial-word prefix matches.
		score += 256
	}
	return score, true
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isWordBoundary(text []rune, idx int) bool {
	if idx == 0 {
		return true
	}
	prev, cur := text[idx-1], text[idx]
	if !isWordRune(prev) {
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(cur)
}
