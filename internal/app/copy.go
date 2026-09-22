package app

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// A selection in a pane's history, in absolute line indexes (see
// vt.Terminal.LineAt) so it holds still while new output arrives.
type selection struct {
	anchorAbs, anchorCol int
	byLine               bool // V: whole lines rather than characters
}

// ordered returns the selection's two ends with the earlier one first.
func (s *scroll) ordered() (fromAbs, fromCol, toAbs, toCol int) {
	sel := s.sel
	fromAbs, fromCol = sel.anchorAbs, sel.anchorCol
	toAbs, toCol = s.curAbs, s.curCol
	if toAbs < fromAbs || (toAbs == fromAbs && toCol < fromCol) {
		fromAbs, fromCol, toAbs, toCol = toAbs, toCol, fromAbs, fromCol
	}
	return
}

// selectedSpan reports which columns of the line with absolute index abs
// are selected. ok is false when the line is outside the selection.
func (s *scroll) selectedSpan(abs, cols int) (from, to int, ok bool) {
	if s.sel == nil {
		return 0, 0, false
	}
	fromAbs, fromCol, toAbs, toCol := s.ordered()
	if abs < fromAbs || abs > toAbs {
		return 0, 0, false
	}
	if s.sel.byLine {
		return 0, cols - 1, true
	}
	from, to = 0, cols-1
	if abs == fromAbs {
		from = fromCol
	}
	if abs == toAbs {
		to = toCol
	}
	return from, to, from <= to
}

// selectedText gathers what the selection covers. Trailing blanks go: a
// terminal line is padded to the width of the pane, and nobody wants to
// paste eighty spaces.
func (a *App) selectedText() string {
	s := a.scroll
	if s == nil || s.sel == nil {
		return ""
	}
	_, _, _, cols := s.pane.Rect()
	fromAbs, _, toAbs, _ := s.ordered()

	var lines []string
	for abs := fromAbs; abs <= toAbs; abs++ {
		from, to, ok := s.selectedSpan(abs, cols)
		if !ok {
			continue
		}
		text := []rune(s.pane.LineText(abs))
		if from >= len(text) {
			lines = append(lines, "")
			continue
		}
		if to >= len(text) {
			to = len(text) - 1
		}
		lines = append(lines, strings.TrimRight(string(text[from:to+1]), " "))
	}
	out := strings.Join(lines, "\n")
	if s.sel.byLine {
		out += "\n"
	}
	return out
}

// osc52 is the escape sequence that asks the terminal to put text on the
// system clipboard. It is the only way that works over ssh, because the
// terminal doing the copying is the one in front of the user.
func osc52(text string) []byte {
	return []byte("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a")
}

// yank copies the selection and leaves copy mode. Called with the lock
// held.
func (a *App) yank() {
	text := a.selectedText()
	a.scroll = nil
	if text == "" {
		return
	}

	if a.cfg.Copy.OSC52 {
		a.screen.WriteRaw(osc52(text))
	}
	// A terminal that refuses OSC 52 (many do, for good reasons) can be
	// worked around with a local command — wl-copy, xclip, pbcopy.
	if cmd := a.cfg.Copy.Command; strings.TrimSpace(cmd) != "" {
		a.pipeTo(cmd, text)
	}

	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	switch {
	case lines > 1:
		a.notify(fmt.Sprintf("copied %d lines", lines))
	default:
		a.notify(fmt.Sprintf("copied %d characters", len([]rune(text))))
	}
	a.markDirty()
}

// pipeTo runs a command with the text on its standard input, the same way
// the [agent] hooks run: detached, output to the daemon's log.
func (a *App) pipeTo(command, text string) {
	c := hookShell(command)
	c.Stdin = strings.NewReader(text)
	c.Stdout, c.Stderr = nil, nil
	if err := c.Start(); err != nil {
		a.notify("copy command: " + err.Error())
		return
	}
	go c.Wait()
}
