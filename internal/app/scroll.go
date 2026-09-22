package app

import (
	"fmt"

	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/ui"
	"github.com/noturbob/slat/internal/vt"
)

// scroll is the state of scroll mode: the active pane showing its history
// instead of its live screen. The view is anchored by absolute line index
// (see vt.Terminal.LineAt), so it holds still while new output arrives.
type scroll struct {
	pane  *pane.Pane
	top   int // absolute index of the line at the top of the view
	query string
	dir   int // direction of the last search: -1 older, +1 newer

	// Where the copy cursor sits, and what it has selected so far. The
	// cursor is what h/j/k/l move; the view follows it.
	curAbs, curCol int
	sel            *selection

	matched                    bool
	matchAbs, matchCol, matchW int
}

var (
	matchStyle     = vt.Style{Fg: vt.Indexed(16), Bg: vt.Indexed(220)}
	selectionStyle = vt.Style{Attrs: vt.Reverse}
	cursorStyle    = vt.Style{Fg: vt.Indexed(16), Bg: vt.Indexed(252)}
)

// enterScroll puts the active pane into scroll mode.
func (a *App) enterScroll() bool {
	p := a.manager.ActivePane()
	_, screenTop, alt := p.History()
	if alt {
		a.notify("scroll back after the full-screen program exits (it keeps no history)")
		return false
	}
	_, _, rows, _ := p.Rect()
	a.scroll = &scroll{pane: p, top: screenTop, curAbs: screenTop + rows - 1}
	return true
}

// scrollBy moves the view n lines towards newer output (negative: older).
func (a *App) scrollBy(n int) {
	s := a.scroll
	first, screenTop, _ := s.pane.History()
	s.top = min(max(s.top+n, first), screenTop)
}

// moveCursor moves the copy cursor and brings the view with it, so the
// cursor never walks off the screen.
func (a *App) moveCursor(dRows, dCols int) {
	s := a.scroll
	first, screenTop, _ := s.pane.History()
	_, _, rows, cols := s.pane.Rect()

	s.curAbs = min(max(s.curAbs+dRows, first), screenTop+rows-1)
	s.curCol = min(max(s.curCol+dCols, 0), max(cols-1, 0))

	switch {
	case s.curAbs < s.top:
		s.top = s.curAbs
	case s.curAbs >= s.top+rows:
		s.top = s.curAbs - rows + 1
	}
	a.scrollBy(0) // clamp the view to the history that still exists
}

// cursorToLineEnd puts the cursor on the last character of its line, not
// in the blank padding after it.
func (a *App) cursorToLineEnd() {
	s := a.scroll
	_, _, _, cols := s.pane.Rect()
	text := []rune(s.pane.LineText(s.curAbs))
	s.curCol = min(max(len(text)-1, 0), max(cols-1, 0))
}

func (a *App) scrollRows() int {
	_, _, rows, _ := a.scroll.pane.Rect()
	return rows
}

// scrollKeys maps keys in scroll mode to actions. Escape sequences are
// listed without their leading ESC.
var scrollKeys = map[string]string{
	"k": "up", "[A": "up", "OA": "up", "\x19": "up", // Ctrl-Y
	"j": "down", "[B": "down", "OB": "down", "\x05": "down", "\r": "down", // Ctrl-E
	"h": "left", "[D": "left", "OD": "left",
	"l": "right", "[C": "right", "OC": "right",
	"0": "line-start", "^": "line-start",
	"$": "line-end",
	"v": "select", "V": "select-lines", "y": "yank",
	"\x15": "half-up", "u": "half-up", // Ctrl-U
	"\x04": "half-down", "d": "half-down", // Ctrl-D
	"\x02": "page-up", "b": "page-up", "[5~": "page-up", // Ctrl-B
	"\x06": "page-down", "f": "page-down", " ": "page-down", "[6~": "page-down", // Ctrl-F
	"g": "top", "[H": "top", "OH": "top", "[1~": "top",
	"G": "bottom", "[F": "bottom", "OF": "bottom", "[4~": "bottom",
	"/": "search-up", "?": "search-down",
	"n": "next", "N": "prev",
	"q": "exit", "\x03": "exit", // Ctrl-C
}

// scrollKey handles the key starting at buf[i] in scroll mode and returns
// the index of its last byte.
func (a *App) scrollKey(buf []byte, i int) int {
	key := string(buf[i])
	if buf[i] == 0x1b {
		switch {
		case i+1 == len(buf):
			// Esc steps back one level: it drops a selection first, and
			// only leaves scroll mode when there is nothing to drop.
			if a.scroll.sel != nil {
				a.scroll.sel = nil
			} else {
				a.scroll = nil
			}
			return i
		case buf[i+1] == '[' || buf[i+1] == 'O':
			j := i + 2
			for j < len(buf) && (buf[j] < 0x40 || buf[j] > 0x7e) {
				j++
			}
			if j == len(buf) {
				return j - 1 // truncated sequence: drop it
			}
			key, i = string(buf[i+1:j+1]), j
		default:
			return i + 1 // Alt+key: not a scroll key
		}
	}

	rows := a.scrollRows()
	switch scrollKeys[key] {
	case "up":
		a.moveCursor(-1, 0)
	case "down":
		a.moveCursor(1, 0)
	case "left":
		a.moveCursor(0, -1)
	case "right":
		a.moveCursor(0, 1)
	case "line-start":
		a.scroll.curCol = 0
	case "line-end":
		a.cursorToLineEnd()
	case "half-up":
		a.moveCursor(-max(rows/2, 1), 0)
	case "half-down":
		a.moveCursor(max(rows/2, 1), 0)
	case "page-up":
		a.moveCursor(-max(rows-1, 1), 0)
	case "page-down":
		a.moveCursor(max(rows-1, 1), 0)
	case "top":
		a.moveCursor(-1<<30, 0)
	case "bottom":
		a.moveCursor(1<<30, 0)
	case "select", "select-lines":
		s := a.scroll
		if s.sel != nil {
			s.sel = nil // pressing it again drops the selection
			break
		}
		s.sel = &selection{
			anchorAbs: s.curAbs,
			anchorCol: s.curCol,
			byLine:    scrollKeys[key] == "select-lines",
		}
	case "yank":
		a.yank()
	case "search-up", "search-down":
		dir := -1
		if key == "?" {
			dir = 1
		}
		label := map[int]string{-1: "search up", 1: "search down"}[dir]
		a.startPrompt(label, "", func(q string) { a.find(q, dir) })
	case "next", "prev":
		if s := a.scroll; s.query != "" {
			dir := s.dir
			if key == "N" {
				dir = -dir
			}
			a.findFrom(s.query, dir)
		}
	case "exit":
		a.scroll = nil
	}
	return i
}

// find starts a new search for query in direction dir.
func (a *App) find(query string, dir int) {
	if a.scroll == nil {
		return
	}
	a.scroll.query, a.scroll.dir, a.scroll.matched = query, dir, false
	a.findFrom(query, dir)
}

// findFrom moves to the next match of query in direction dir, starting
// next to the current match, or from the edge of the view.
func (a *App) findFrom(query string, dir int) {
	s := a.scroll
	rows := a.scrollRows()
	from := s.top
	switch {
	case s.matched:
		from = s.matchAbs + dir
	case dir < 0:
		from = s.top + rows - 1
	}
	abs, col, w, ok := s.pane.Search(query, from, dir)
	if !ok {
		a.notify("not found: " + query)
		return
	}
	s.matched, s.matchAbs, s.matchCol, s.matchW = true, abs, col, w
	// The cursor follows the match, so a search is how you get the copy
	// cursor to the line you want without pressing k forty times.
	s.curAbs, s.curCol = abs, col
	if abs < s.top || abs >= s.top+rows {
		s.top = abs - rows/3 // show the match with some context above it
		a.scrollBy(0)        // clamp
	}
}

// drawScroll draws the scrolled pane and its search match into frame.
func (a *App) drawScroll(frame *ui.Frame) {
	s := a.scroll
	a.scrollBy(0) // history may have been trimmed since the last frame
	s.pane.DrawHistory(frame.Lines, s.top)
	pr, pc, rows, cols := s.pane.Rect()

	// The selection first, so a search hit inside it still stands out.
	for y := 0; y < rows; y++ {
		from, to, ok := s.selectedSpan(s.top+y, cols)
		if !ok {
			continue
		}
		for x := from; x <= to && x < cols; x++ {
			if pr+y < frame.H && pc+x < frame.W {
				frame.Lines[pr+y][pc+x].Style = selectionStyle
			}
		}
	}

	if y := s.matchAbs - s.top; s.matched && y >= 0 && y < rows {
		for x := s.matchCol; x < s.matchCol+s.matchW && x < cols; x++ {
			if pr+y < frame.H && pc+x < frame.W {
				frame.Lines[pr+y][pc+x].Style = matchStyle
			}
		}
	}

	// The copy cursor: slat draws it itself, because the terminal's own
	// cursor is parked with the live screen.
	if y := s.curAbs - s.top; y >= 0 && y < rows && s.curCol < cols &&
		pr+y < frame.H && pc+s.curCol < frame.W {
		frame.Lines[pr+y][pc+s.curCol].Style = cursorStyle
	}
}

// scrollBadge is the status bar label: lines above the live screen, out
// of the history kept.
func (a *App) scrollBadge() string {
	s := a.scroll
	first, screenTop, _ := s.pane.History()
	if s.sel != nil {
		fromAbs, _, toAbs, _ := s.ordered()
		if s.sel.byLine || toAbs > fromAbs {
			return "COPY " + plural(toAbs-fromAbs+1, "line")
		}
		return "COPY"
	}
	return fmt.Sprintf("SCROLL %d/%d", screenTop-s.top, screenTop-first)
}
