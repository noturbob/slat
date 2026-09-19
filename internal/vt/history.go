package vt

import "unicode"

// Scrollback: lines scrolled off the top of the main screen are kept in a
// ring. Lines are addressed by absolute index: history occupies
// [FirstLine, Pushed) and the main screen's row y is Pushed+y. An index
// keeps naming the same line as more output arrives, which is what lets a
// scrolled-back view stay still.

// SetScrollback sets how many lines of history to keep (0 disables it).
func (t *Terminal) SetScrollback(n int) {
	t.histMax = max(n, 0)
	t.ClearHistory()
}

// ClearHistory drops all scrollback.
func (t *Terminal) ClearHistory() {
	t.hist, t.histStart, t.histLen = nil, 0, 0
}

// Pushed is the absolute index of the main screen's first row, i.e. the
// number of lines that have ever entered history.
func (t *Terminal) Pushed() int { return t.pushed }

// FirstLine is the absolute index of the oldest line still in history.
func (t *Terminal) FirstLine() int { return t.pushed - t.histLen }

// LineAt returns the line at absolute index abs, from history or the main
// screen, or nil if it's out of range. History lines are trimmed of
// trailing blanks, so they may be shorter than the screen.
func (t *Terminal) LineAt(abs int) []Cell {
	switch {
	case abs >= t.pushed && abs-t.pushed < t.rows:
		return t.main[abs-t.pushed]
	case abs >= t.FirstLine() && abs < t.pushed:
		return t.hist[(t.histStart+abs-t.FirstLine())%t.histMax]
	}
	return nil
}

// pushHistory saves a copy of line, which is leaving the main screen.
func (t *Terminal) pushHistory(line []Cell) {
	if t.histMax == 0 {
		return
	}
	n := len(line)
	for n > 0 {
		// Field checks rather than == Cell{}: this runs for every cell of
		// every scrolled line, and the struct compare isn't inlined.
		c := &line[n-1]
		if c.R != 0 || c.Style != (Style{}) || c.Wide != 0 || len(c.Comb) != 0 {
			break
		}
		n--
	}
	saved := append([]Cell(nil), line[:n]...)
	switch {
	case t.histLen < t.histMax:
		t.hist = append(t.hist, saved)
		t.histLen++
	default: // full: overwrite the oldest
		t.hist[t.histStart] = saved
		t.histStart = (t.histStart + 1) % t.histMax
	}
	t.pushed++
}

// Search looks for query in lines from absolute index from, moving by dir
// (-1 towards older lines, +1 towards newer), through history and the main
// screen. A query with no capitals matches any case. It returns the line,
// the column where the match starts and its width in columns.
func (t *Terminal) Search(query string, from, dir int) (abs, col, width int, ok bool) {
	q := []rune(query)
	fold := true
	for _, r := range q {
		if unicode.IsUpper(r) {
			fold = false
		}
	}
	if fold {
		for i, r := range q {
			q[i] = unicode.ToLower(r)
		}
	}
	if len(q) == 0 {
		return 0, 0, 0, false
	}
	for abs = from; abs >= t.FirstLine() && abs < t.pushed+t.rows; abs += dir {
		var text []rune
		var cols []int
		for x, c := range t.LineAt(abs) {
			if c.Wide == WideTail {
				continue
			}
			r := c.R
			if r == 0 {
				r = ' '
			}
			if fold {
				r = unicode.ToLower(r)
			}
			text = append(text, r)
			cols = append(cols, x)
		}
	next:
		for i := 0; i+len(q) <= len(text); i++ {
			for j, r := range q {
				if text[i+j] != r {
					continue next
				}
			}
			end := cols[i+len(q)-1] + 1
			if RuneWidth(text[i+len(q)-1]) == 2 {
				end++
			}
			return abs, cols[i], end - cols[i], true
		}
	}
	return 0, 0, 0, false
}
