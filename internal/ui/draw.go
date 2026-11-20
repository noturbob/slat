package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/vt"
)

var (
	styleBar          = vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	styleWorkspace    = vt.Style{Fg: vt.Indexed(114), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleTab          = vt.Style{Fg: vt.Indexed(250), Bg: vt.Indexed(236)}
	styleTabActive    = vt.Style{Fg: vt.Indexed(231), Bg: vt.Indexed(25), Attrs: vt.Bold}
	styleBadge        = vt.Style{Fg: vt.Indexed(16), Bg: vt.Indexed(114), Attrs: vt.Bold}
	styleDim          = vt.Style{Fg: vt.Indexed(248), Bg: vt.Indexed(235)} // ≥4.5:1 on 235
	styleAccent       = vt.Indexed(44)
	stylePrefix       = vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	stylePrefixActive = vt.Style{Fg: vt.Indexed(16), Bg: vt.Indexed(114), Attrs: vt.Bold}
	styleBorder       = vt.Style{Fg: vt.Indexed(240)}
	styleBorderOn  = vt.Style{Fg: vt.Indexed(44), Attrs: vt.Bold}
	styleBox       = vt.Style{Fg: vt.Indexed(44), Bg: vt.Indexed(235)}
	styleBoxKey    = vt.Style{Fg: vt.Indexed(231), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleBoxText   = vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	styleBoxHead     = vt.Style{Fg: vt.Indexed(44), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleFinder      = styleBox
	styleFinderKey   = styleBoxKey
	styleFinderText  = styleBoxText
	styleFinderHead  = styleBoxHead
	styleFinderSel   = styleTabActive
	styleStack       = styleTab
	styleStackActive = styleTabActive
	styleBanner      = vt.Style{Fg: vt.Indexed(44), Attrs: vt.Bold}
	styleBannerSub = vt.Style{Fg: vt.Indexed(244)}
)

// Text draws s at (x, y), clipped to the frame, and returns the column
// after it.
func Text(f *Frame, x, y int, s string, st vt.Style) int {
	prevX := -1
	for _, r := range s {
		switch vt.RuneWidth(r) {
		case 0:
			if prevX >= 0 && prevX < f.W && y >= 0 && y < f.H && r >= 0x20 {
				f.Lines[y][prevX].Comb += string(r)
			}
		case 1:
			f.Set(x, y, vt.Cell{R: r, Style: st})
			prevX = x
			x++
		case 2:
			f.Set(x, y, vt.Cell{R: r, Wide: vt.WideHead, Style: st})
			f.Set(x+1, y, vt.Cell{Wide: vt.WideTail, Style: st})
			prevX = x
			x += 2
		}
	}
	return x
}

// Fill paints n cells from (x, y) with blanks in style st.
func Fill(f *Frame, x, y, n int, st vt.Style) {
	for i := 0; i < n; i++ {
		f.Set(x+i, y, vt.Cell{Style: st})
	}
}

// Truncate cuts s to at most w columns, marking the cut with "…".
func Truncate(s string, w int) string {
	if vt.StringWidth(s) <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	return vt.Truncate(s, w, "…")
}

// ─── Borders ────────────────────────────────────────────────────────────────

// Border glyphs indexed by which neighbors are also border cells.
const (
	up = 1 << iota
	down
	left
	right
)

// StackHeader describes one row in the stacked-mode header list.
type StackHeader struct {
	Index  int
	Title  string
	Active bool
}

// DrawStackHeaders draws 1 row per stacked pane at the top of area.
func DrawStackHeaders(f *Frame, area layout.Rect, headers []StackHeader) {
	for i, h := range headers {
		if i >= area.Rows {
			break
		}
		y := area.Row + i
		st := styleStack
		prefix := "   "
		if h.Active {
			st = styleStackActive
			prefix = " ▸ "
		}
		Fill(f, area.Col, y, area.Cols, st)
		line := fmt.Sprintf("%s%d: %s", prefix, h.Index, h.Title)
		Text(f, area.Col, y, Truncate(line, area.Cols), st)
	}
}

// DrawBorders draws the separators between panes, joining where they meet,
// and highlights the ones around the active pane.
func DrawBorders(f *Frame, borders []layout.Rect, active layout.Rect) {
	cells := map[[2]int]bool{}
	for _, r := range borders {
		for y := r.Row; y < r.Row+r.Rows; y++ {
			for x := r.Col; x < r.Col+r.Cols; x++ {
				cells[[2]int{x, y}] = true
			}
		}
	}
	around := layout.Rect{Row: active.Row - 1, Col: active.Col - 1, Rows: active.Rows + 2, Cols: active.Cols + 2}
	for c := range cells {
		x, y := c[0], c[1]
		mask := 0
		if cells[[2]int{x, y - 1}] {
			mask |= up
		}
		if cells[[2]int{x, y + 1}] {
			mask |= down
		}
		if cells[[2]int{x - 1, y}] {
			mask |= left
		}
		if cells[[2]int{x + 1, y}] {
			mask |= right
		}
		g := borderSet.glyph(mask)
		st := styleBorder
		if around.Contains(y, x) {
			st = styleBorderOn
		}
		f.Set(x, y, vt.Cell{R: g, Style: st})
	}
}

// ─── Status bar ─────────────────────────────────────────────────────────────

// Status is what the status bar shows.
type Status struct {
	Session        string
	Workspace      string
	WorkspaceIndex int // 0-based
	WorkspaceCount int
	Tabs           []string
	TabAlert       []bool // a pane in this tab is waiting for input
	TabPaneCount   []int  // number of panes in each tab
	TabActivePane  []int  // 0-based active pane index in each tab
	ActiveTab      int
	Badge          string // "PREFIX", "ZOOM", ... or ""
	PrefixActive   bool
	Zoomed         bool
	PaneIndex      int    // 0-based
	PaneCount      int
	Message        string // shown instead of the tabs while set
}

// DrawStatusBar draws the status bar on row y, laid out by f. The right
// side wins when the two sides don't fit: a bar that hides which pane you
// are in is worse than one with a truncated tab list.
func DrawStatusBar(f *Frame, y int, st Status, format StatusFormat) {
	if st.Badge == "PREFIX" && format.HasPrefixPlaceholder() {
		st.Badge = ""
	}
	if st.Badge == "ZOOM" && format.HasZoomIndicator() {
		st.Badge = ""
	}
	w := f.W
	Fill(f, 0, y, w, styleBar)

	rightSink := barSink{}
	emitStatus(&rightSink, format.Right, st, format)
	rightW := rightSink.x
	if rightW > w {
		rightW = 0
	}

	leftSink := barSink{frame: f, x: 0, y: y, budget: w - rightW}
	if st.Message != "" {
		leftSink.emit(" ", styleBar)
		leftSink.emit(st.Message, styleBar)
	} else {
		emitStatus(&leftSink, format.Left, st, format)
	}

	if rightW > 0 {
		rightDraw := barSink{frame: f, x: w - rightW, y: y, budget: w}
		emitStatus(&rightDraw, format.Right, st, format)
	}
}

// DrawPrompt draws an input line on row y, returning the cursor column.
func DrawPrompt(f *Frame, y int, label, text string) int {
	w := f.W
	Fill(f, 0, y, w, styleBadge)
	head := " " + label + ": "
	// Keep the end of the text (where the user is typing) visible.
	room := w - vt.StringWidth(head) - 1
	for room > 0 && vt.StringWidth(text) > room {
		_, size := utf8.DecodeRuneInString(text)
		text = text[size:]
	}
	x := Text(f, 0, y, head, styleBadge)
	return Text(f, x, y, text, styleBadge)
}

// DrawDialog draws a centered modal input box for renaming sessions, tabs, or
// workspaces, returning the cursor position (x, y). Falls back to DrawPrompt
// on tiny terminals.
func DrawDialog(f *Frame, title, text string) (int, int) {
	if f.W < 24 || f.H < 7 {
		return DrawPrompt(f, f.H-1, title, text), f.H - 1
	}
	boxW := min(max(vt.StringWidth(title)+8, 42), f.W)
	const boxH = 5
	x0, y0 := (f.W-boxW)/2, (f.H-boxH)/2

	for y := 0; y < boxH; y++ {
		Fill(f, x0, y0+y, boxW, styleBox)
	}
	Text(f, x0, y0, "╭"+strings.Repeat("─", boxW-2)+"╮", styleBox)
	for y := 1; y < boxH-1; y++ {
		Text(f, x0, y0+y, "│", styleBox)
		Text(f, x0+boxW-1, y0+y, "│", styleBox)
	}
	Text(f, x0, y0+boxH-1, "╰"+strings.Repeat("─", boxW-2)+"╯", styleBox)

	head := " " + title + " "
	Text(f, x0+(boxW-vt.StringWidth(head))/2, y0, head, styleBoxHead)

	// Input row with a subtle prompt marker and visible tail of text.
	const promptMark = "> "
	innerW := boxW - 4 - len(promptMark) - 1
	for innerW > 0 && vt.StringWidth(text) > innerW {
		_, size := utf8.DecodeRuneInString(text)
		text = text[size:]
	}
	ix := Text(f, x0+2, y0+2, promptMark, styleBoxHead)
	curX := Text(f, ix, y0+2, text, styleBoxKey)

	foot := " Enter save · Esc cancel "
	if vt.StringWidth(foot) <= boxW-2 {
		Text(f, x0+(boxW-vt.StringWidth(foot))/2, y0+boxH-1, foot, styleBoxText)
	}
	return curX, y0 + 2
}

// TreeRow is one visible entry in DrawTreePicker.
type TreeRow struct {
	Label    string
	Selected bool
	Active   bool
}

// DrawTreePicker draws a centered fuzzy tree picker modal, returning the
// cursor position (x, y) at the end of the filter query.
func DrawTreePicker(f *Frame, title, query string, rows []TreeRow, offset, total int) (int, int) {
	boxW := min(max(f.W-4, 28), 76)
	if boxW > f.W {
		boxW = f.W
	}
	boxH := min(max(f.H-4, 8), 20)
	if boxH > f.H {
		boxH = f.H
	}
	x0, y0 := (f.W-boxW)/2, (f.H-boxH)/2

	for y := 0; y < boxH; y++ {
		Fill(f, x0, y0+y, boxW, styleFinder)
	}
	Text(f, x0, y0, "╭"+strings.Repeat("─", max(boxW-2, 0))+"╮", styleFinder)
	for y := 1; y < boxH-1; y++ {
		Text(f, x0, y0+y, "│", styleFinder)
		Text(f, x0+boxW-1, y0+y, "│", styleFinder)
	}
	Text(f, x0, y0+boxH-1, "╰"+strings.Repeat("─", max(boxW-2, 0))+"╯", styleFinder)

	head := fmt.Sprintf(" %s (%d/%d) ", title, len(rows), total)
	if vt.StringWidth(head) <= boxW-2 {
		Text(f, x0+(boxW-vt.StringWidth(head))/2, y0, head, styleFinderHead)
	}

	// Input row.
	const promptMark = "> "
	innerW := boxW - 4 - len(promptMark) - 1
	q := query
	for innerW > 0 && vt.StringWidth(q) > innerW {
		_, size := utf8.DecodeRuneInString(q)
		q = q[size:]
	}
	ix := Text(f, x0+2, y0+1, promptMark, styleFinderHead)
	curX := Text(f, ix, y0+1, q, styleFinderKey)

	if boxH > 4 {
		Text(f, x0, y0+2, "├"+strings.Repeat("─", max(boxW-2, 0))+"┤", styleFinder)
	}

	listY := y0 + 3
	listH := max(boxH-4, 0)
	for i := 0; i < listH; i++ {
		idx := offset + i
		if idx >= len(rows) {
			break
		}
		r := rows[idx]
		st := styleFinderText
		mark := "  "
		if r.Selected {
			st = styleFinderSel
			mark = "▸ "
		} else if r.Active {
			st = styleFinderHead
		}
		Fill(f, x0+1, listY+i, boxW-2, st)
		line := Truncate(mark+r.Label, boxW-4)
		Text(f, x0+2, listY+i, line, st)
	}

	foot := " ↑↓/C-n/C-p move · Enter jump · Esc cancel "
	if vt.StringWidth(foot) <= boxW-2 {
		Text(f, x0+(boxW-vt.StringWidth(foot))/2, y0+boxH-1, foot, styleFinderText)
	}
	return curX, y0 + 1
}

// ─── Help overlay ───────────────────────────────────────────────────────────

// HelpEntry is one line of the help overlay; an entry with no Key is a
// section heading.
type HelpEntry struct{ Key, Desc string }

// DrawHelp draws the keybinding reference centered on the screen, in two
// columns when one doesn't fit.
func DrawHelp(f *Frame, title string, entries []HelpEntry) {
	const colW = 34
	w, h := f.W, f.H

	columns := [][]HelpEntry{entries}
	if len(entries)+4 > h && w >= 2*colW+3 {
		// Split at the section heading nearest the middle.
		cut := len(entries) / 2
		for d := 0; d < len(entries)/2; d++ {
			if cut+d < len(entries) && entries[cut+d].Key == "" {
				cut += d
				break
			}
			if cut-d > 0 && entries[cut-d].Key == "" {
				cut -= d
				break
			}
		}
		columns = [][]HelpEntry{entries[:cut], entries[cut:]}
	}
	rows := 0
	for _, c := range columns {
		rows = max(rows, len(c))
	}
	boxW := len(columns)*colW + 2 + (len(columns) - 1)
	boxH := rows + 4
	x0, y0 := max((w-boxW)/2, 0), max((h-boxH)/2, 0)

	for y := 0; y < boxH; y++ {
		Fill(f, x0, y0+y, boxW, styleBox)
	}
	Text(f, x0, y0, "╭"+strings.Repeat("─", boxW-2)+"╮", styleBox)
	for y := 1; y < boxH-1; y++ {
		Text(f, x0, y0+y, "│", styleBox)
		Text(f, x0+boxW-1, y0+y, "│", styleBox)
	}
	Text(f, x0, y0+boxH-1, "╰"+strings.Repeat("─", boxW-2)+"╯", styleBox)
	Text(f, x0+(boxW-vt.StringWidth(title))/2, y0+1, title, styleBoxHead)

	for ci, col := range columns {
		cx := x0 + 1 + ci*(colW+1)
		for i, e := range col {
			y := y0 + 3 + i
			if e.Key == "" {
				Text(f, cx+1, y, Truncate("── "+e.Desc+" "+strings.Repeat("─", colW), colW-2), styleBoxHead)
				continue
			}
			Text(f, cx+2, y, e.Key, styleBoxKey)
			Text(f, cx+9, y, Truncate(e.Desc, colW-10), styleBoxText)
		}
	}
	foot := "press any key to close"
	if y0+boxH-1 < h {
		Text(f, x0+(boxW-len(foot))/2, y0+boxH-1, " "+foot+" ", styleBoxText)
	}
}

// ─── Banner ─────────────────────────────────────────────────────────────────

var bannerArt = []string{
	`     _____ __      ___  _______`,
	`    / ___// /     /   |/_  __/`,
	`    \__ \/ /     / /| | / /   `,
	`   ___/ / /___  / ___ |/ /    `,
	`  /____/_____/ /_/  |_/_/     `,
}

// Splash describes a customized startup splash screen.
type Splash struct {
	Art      []string
	Subtitle string
	Hint     string
	Style    vt.Style
	SubStyle vt.Style
	Bg       vt.Color
}

// ParseSplash builds and validates a Splash from config strings. Empty fg/subFg
// keep the active theme's banner colours; bg accepts ""/"transparent" for default.
func ParseSplash(art []string, subtitle, hint, fg, subFg, bg string) (Splash, error) {
	sp := Splash{
		Art:      art,
		Subtitle: subtitle,
		Hint:     hint,
		Style:    styleBanner,
		SubStyle: styleBannerSub,
	}
	if len(sp.Art) == 0 && art == nil {
		sp.Art = bannerArt
	}
	if fg != "" {
		c, err := ParseColor(fg)
		if err != nil {
			return sp, fmt.Errorf("splash.fg: %w", err)
		}
		sp.Style.Fg = c
	}
	if subFg != "" {
		c, err := ParseColor(subFg)
		if err != nil {
			return sp, fmt.Errorf("splash.sub_fg: %w", err)
		}
		sp.SubStyle.Fg = c
	}
	if bg != "" && !isTransparent(bg) {
		c, err := ParseColor(bg)
		if err != nil {
			return sp, fmt.Errorf("splash.bg: %w", err)
		}
		sp.Bg = c
		sp.Style.Bg = c
		sp.SubStyle.Bg = c
	}
	return sp, nil
}

// DrawBanner clears the screen and draws the default startup banner.
func DrawBanner(f *Frame, version, hint string) {
	DrawSplash(f, Splash{
		Art:      bannerArt,
		Subtitle: "terminal multiplexer " + version,
		Hint:     hint,
		Style:    styleBanner,
		SubStyle: styleBannerSub,
	})
}

// DrawSplash clears the screen and draws sp centered in f.
func DrawSplash(f *Frame, sp Splash) {
	if sp.Bg != 0 {
		bgStyle := vt.Style{Bg: sp.Bg}
		for y := 0; y < f.H; y++ {
			Fill(f, 0, y, f.W, bgStyle)
		}
	} else {
		clear(f.cells)
	}
	w, h := f.W, f.H
	lines := 0
	if sp.Subtitle != "" {
		lines++
	}
	if sp.Hint != "" {
		lines++
	}
	totalH := len(sp.Art)
	if lines > 0 && len(sp.Art) > 0 {
		totalH += 1 + lines
	} else {
		totalH += lines
	}
	y := max((h-totalH)/2, 0)
	maxArtW := 0
	for _, l := range sp.Art {
		if lw := vt.StringWidth(l); lw > maxArtW {
			maxArtW = lw
		}
	}
	xArt := max((w-maxArtW)/2, 0)
	for _, l := range sp.Art {
		Text(f, xArt, y, l, sp.Style)
		y++
	}
	if len(sp.Art) > 0 && lines > 0 {
		y++
	}
	for _, s := range []string{sp.Subtitle, sp.Hint} {
		if s == "" {
			continue
		}
		Text(f, max((w-vt.StringWidth(s))/2, 0), y, s, sp.SubStyle)
		y++
	}
}

// Curtain hides the part of r that a new pane hasn't revealed yet, so a
// split slides into place instead of appearing all at once. revealed is
// how far the reveal has got: columns for a left/right split, rows for a
// top/bottom one.
func Curtain(f *Frame, r layout.Rect, sideways bool, revealed int, glyph rune) {
	for y := r.Row; y < r.Row+r.Rows; y++ {
		for x := r.Col; x < r.Col+r.Cols; x++ {
			if sideways && x-r.Col < revealed || !sideways && y-r.Row < revealed {
				continue
			}
			f.Set(x, y, vt.Cell{R: glyph, Style: styleBorder})
		}
	}
}
