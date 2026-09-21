package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/vt"
)

var (
	styleBar       = vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	styleWorkspace = vt.Style{Fg: vt.Indexed(114), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleTab       = vt.Style{Fg: vt.Indexed(250), Bg: vt.Indexed(236)}
	styleTabActive = vt.Style{Fg: vt.Indexed(231), Bg: vt.Indexed(25), Attrs: vt.Bold}
	styleBadge     = vt.Style{Fg: vt.Indexed(16), Bg: vt.Indexed(114), Attrs: vt.Bold}
	styleDim       = vt.Style{Fg: vt.Indexed(248), Bg: vt.Indexed(235)} // ≥4.5:1 on 235
	styleBorder    = vt.Style{Fg: vt.Indexed(240)}
	styleBorderOn  = vt.Style{Fg: vt.Indexed(44), Attrs: vt.Bold}
	styleBox       = vt.Style{Fg: vt.Indexed(44), Bg: vt.Indexed(235)}
	styleBoxKey    = vt.Style{Fg: vt.Indexed(231), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleBoxText   = vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	styleBoxHead   = vt.Style{Fg: vt.Indexed(44), Bg: vt.Indexed(235), Attrs: vt.Bold}
	styleBanner    = vt.Style{Fg: vt.Indexed(44), Attrs: vt.Bold}
	styleBannerSub = vt.Style{Fg: vt.Indexed(244)}
)

// Text draws s at (x, y), clipped to the frame, and returns the column
// after it.
func Text(f *Frame, x, y int, s string, st vt.Style) int {
	for _, r := range s {
		switch vt.RuneWidth(r) {
		case 1:
			f.Set(x, y, vt.Cell{R: r, Style: st})
			x++
		case 2:
			f.Set(x, y, vt.Cell{R: r, Wide: vt.WideHead, Style: st})
			f.Set(x+1, y, vt.Cell{Wide: vt.WideTail, Style: st})
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
	Workspace      string
	WorkspaceIndex int // 0-based
	WorkspaceCount int
	Tabs           []string
	TabAlert       []bool // a pane in this tab is waiting for input
	ActiveTab      int
	Badge          string // "PREFIX", "ZOOM", ... or ""
	PaneIndex      int    // 0-based
	PaneCount      int
	Message        string // shown instead of the tabs while set
}

// DrawStatusBar draws the status bar on row y, laid out by f. The right
// side wins when the two sides don't fit: a bar that hides which pane you
// are in is worse than one with a truncated tab list.
func DrawStatusBar(f *Frame, y int, st Status, format StatusFormat) {
	w := f.W
	Fill(f, 0, y, w, styleBar)

	right := renderStatus(format.Right, st, format)
	rightW := width(right)
	if rightW > w {
		right, rightW = nil, 0
	}
	left := renderStatus(format.Left, st, format)
	if st.Message != "" {
		left = []segment{{text: " " + st.Message, style: styleBar}}
	}

	x := 0
	budget := w - rightW
	for _, seg := range left {
		if x >= budget {
			break
		}
		text := seg.text
		if x+vt.StringWidth(text) > budget {
			text = Truncate(text, budget-x)
		}
		x = Text(f, x, y, text, seg.style)
	}

	x = w - rightW
	for _, seg := range right {
		x = Text(f, x, y, seg.text, seg.style)
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

// DrawBanner clears the screen and draws the startup banner.
func DrawBanner(f *Frame, version, hint string) {
	clear(f.cells)
	w, h := f.W, f.H
	y := max((h-len(bannerArt)-3)/2, 0)
	for _, l := range bannerArt {
		Text(f, max((w-len(bannerArt[0]))/2, 0), y, l, styleBanner)
		y++
	}
	y++
	for _, s := range []string{"terminal multiplexer " + version, hint} {
		Text(f, max((w-vt.StringWidth(s))/2, 0), y, s, styleBannerSub)
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
