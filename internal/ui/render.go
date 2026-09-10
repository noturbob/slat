package ui

import (
	"fmt"
	"strings"

	"github.com/noturbob/slat/internal/session"
)

const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	BgBlue    = "\033[44m"
	BgGreen   = "\033[42m"
	BgGray    = "\033[100m"
	BgDark    = "\033[48;5;235m"
	BgDarkAlt = "\033[48;5;236m"
	FgWhite   = "\033[97m"
	FgBlack   = "\033[30m"
	FgGray    = "\033[90m"
	FgCyan    = "\033[36m"
	FgYellow  = "\033[33m"
	FgGreen   = "\033[32m"

	HideCursor = "\033[?25l"
	ShowCursor = "\033[?25h"
	SaveCursor = "\0337"
	RestCursor = "\0338"
	Clear      = "\033[2J\033[H"

	BoxH  = "\u2500"
	BoxV  = "\u2502"
	BoxTL = "\u250c"
	BoxTR = "\u2510"
	BoxBL = "\u2514"
	BoxBR = "\u2518"
)

func SetScrollRegion(top, bottom int) string {
	return fmt.Sprintf("\033[%d;%dr", top, bottom)
}

func ResetScrollRegion() string {
	return "\033[r"
}

// EnableHMargins/DisableHMargins toggle DECLRMM (left/right margin mode).
// DECSLRM (SetHMargins) is only honored by the terminal while this mode is
// on, and while it's on, a bare "CSI s" from pane content is reinterpreted
// as DECSLRM instead of the (rarely used) ANSI save-cursor. To keep that
// window as small as possible, callers should enable margins, set them,
// write, then reset margins to full width and disable immediately after --
// mirroring how SetScrollRegion is already toggled around each pane write.
func EnableHMargins() string {
	return "\033[?69h"
}

func DisableHMargins() string {
	return "\033[?69l"
}

// SetHMargins confines cursor addressing, autowrap, and scrolling to the
// column range [left, right] (1-based, inclusive) via DECSLRM. Panes are
// rendered by streaming raw shell output directly onto the real terminal
// with no per-pane screen buffer, so nothing stops a shell that thinks its
// window is narrower than the physical terminal from auto-wrapping (or
// simply printing) past its pane's right edge into a neighboring pane.
// DECSLRM makes the real terminal enforce the same width the pane's shell
// was told it has, so wrapping happens at the pane boundary instead of
// bleeding into whatever pane happens to sit to its right.
func SetHMargins(left, right int) string {
	return fmt.Sprintf("\033[%d;%ds", left, right)
}

// bannerArt is the plain ASCII-art lines of the startup banner, kept
// separate from ANSI styling so their visible width can be measured for
// centering.
var bannerArt = []string{
	`     _____ __      ___  _______`,
	`    / ___// /     /   |/_  __/`,
	`    \__ \/ /     / /| | / /`,
	`   ___/ / /___  / ___ |/ /`,
	`  /____/_____/ /_/  |_/_/`,
}

// GetBanner renders the startup ASCII banner, centered for a terminal that
// is cols columns wide.
func GetBanner(cols int) string {
	width := 0
	for _, l := range bannerArt {
		if len(l) > width {
			width = len(l)
		}
	}
	pad := (cols - width) / 2
	if pad < 0 {
		pad = 0
	}
	indent := strings.Repeat(" ", pad)

	var b strings.Builder
	b.WriteString(FgCyan)
	b.WriteString(Bold)
	for _, l := range bannerArt {
		b.WriteString(indent)
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	b.WriteString(Reset)
	b.WriteString("\r\n")

	for _, sub := range []string{"Terminal Multiplexer v0.1.0", "Press Ctrl-S then ? for help"} {
		subPad := (cols - len(sub)) / 2
		if subPad < 0 {
			subPad = 0
		}
		b.WriteString(Dim)
		b.WriteString(strings.Repeat(" ", subPad))
		b.WriteString(sub)
		b.WriteString(Reset)
		b.WriteString("\r\n")
	}

	return b.String()
}

// DrawStatusBar renders the status bar. Width-budgeted so tabs never
// collide with the right-hand mode/pane indicator on narrow terminals.
func DrawStatusBar(cols, rows int, mode string, manager *session.Manager, prefixActive bool) string {
	if rows < 2 || cols < 2 {
		return ""
	}
	ws := manager.GetCurrentWorkspace()
	if ws == nil {
		return ""
	}

	var bar strings.Builder
	bar.WriteString(SaveCursor)
	bar.WriteString(HideCursor)
	bar.WriteString(fmt.Sprintf("\033[%d;1H", rows))
	bar.WriteString("\033[2K")
	// LINT FIX: was bar.WriteString(BgDark + FgWhite)
	bar.WriteString(BgDark)
	bar.WriteString(FgWhite)

	activePane := manager.GetActivePane()
	paneID := 0
	if activePane != nil {
		paneID = activePane.ID
	}

	var right strings.Builder
	rightVisLen := 0
	if prefixActive {
		// LINT FIX: split concatenated WriteString into sequential calls
		right.WriteString(BgGreen)
		right.WriteString(FgBlack)
		right.WriteString(Bold)
		right.WriteString(" PREFIX ")
		right.WriteString(Reset)
		right.WriteString(BgDark)
		right.WriteString(" ")
		rightVisLen += 9
	} else if mode != "" && mode != "NORMAL" {
		label := " " + mode + " "
		right.WriteString(BgGreen)
		right.WriteString(FgBlack)
		right.WriteString(Bold)
		right.WriteString(label)
		right.WriteString(Reset)
		right.WriteString(BgDark)
		right.WriteString(" ")
		rightVisLen += len(label) + 1
	}
	paneLabel := fmt.Sprintf("Pn:%d ", paneID)
	// LINT FIX: was right.WriteString(FgGray + paneLabel)
	right.WriteString(FgGray)
	right.WriteString(paneLabel)
	rightVisLen += len(paneLabel)

	leftBudget := cols - rightVisLen - 1
	if leftBudget < 1 {
		leftBudget = 1
	}

	var left strings.Builder
	leftVisLen := 0

	wsLabel := fmt.Sprintf("[%s]", ws.Name)
	// LINT FIX: was left.WriteString(" " + FgGreen + Bold + BgDark + wsLabel + Reset + BgDark + FgWhite)
	left.WriteString(" ")
	left.WriteString(FgGreen)
	left.WriteString(Bold)
	left.WriteString(BgDark)
	left.WriteString(wsLabel)
	left.WriteString(Reset)
	left.WriteString(BgDark)
	left.WriteString(FgWhite)
	leftVisLen += 1 + len(wsLabel)

	// LINT FIX: was left.WriteString(" " + FgGray + BoxV + Reset + BgDark + FgWhite + " ")
	left.WriteString(" ")
	left.WriteString(FgGray)
	left.WriteString(BoxV)
	left.WriteString(Reset)
	left.WriteString(BgDark)
	left.WriteString(FgWhite)
	left.WriteString(" ")
	leftVisLen += 3

	for i, tab := range ws.Tabs {
		name := fmt.Sprintf(" %d:%s ", i+1, tab.Name)
		if leftVisLen+len(name) > leftBudget {
			if leftVisLen < leftBudget {
				// LINT FIX: was left.WriteString(FgGray + "…" + Reset + BgDark)
				left.WriteString(FgGray)
				left.WriteString("…")
				left.WriteString(Reset)
				left.WriteString(BgDark)
				leftVisLen++
			}
			break
		}
		if i == ws.ActiveTabIdx {
			// LINT FIX: was left.WriteString(BgBlue + FgWhite + Bold + name + Reset + BgDark)
			left.WriteString(BgBlue)
			left.WriteString(FgWhite)
			left.WriteString(Bold)
			left.WriteString(name)
			left.WriteString(Reset)
			left.WriteString(BgDark)
		} else {
			// LINT FIX: was left.WriteString(BgDarkAlt + FgGray + name + Reset + BgDark)
			left.WriteString(BgDarkAlt)
			left.WriteString(FgGray)
			left.WriteString(name)
			left.WriteString(Reset)
			left.WriteString(BgDark)
		}
		left.WriteString(" ")
		leftVisLen += len(name) + 1
	}

	bar.WriteString(left.String())

	rightPos := cols - rightVisLen + 1
	if rightPos < leftVisLen+2 {
		rightPos = leftVisLen + 2
	}
	if rightPos >= 1 && rightPos <= cols {
		bar.WriteString(fmt.Sprintf("\033[%d;%dH", rows, rightPos))
		bar.WriteString(right.String())
	}

	bar.WriteString(Reset)
	bar.WriteString(RestCursor)
	bar.WriteString(ShowCursor)
	return bar.String()
}

func MoveCursorToPane(row, col int) string {
	return fmt.Sprintf("\033[%d;%dH", row, col)
}

func DrawVerticalBorder(row, col, height int) string {
	var b strings.Builder
	b.WriteString(SaveCursor)
	for i := 0; i < height; i++ {
		b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row+i, col, FgGray+Dim, BoxV, Reset))
	}
	b.WriteString(RestCursor)
	return b.String()
}

func DrawHorizontalBorder(row, col, width int) string {
	var b strings.Builder
	b.WriteString(SaveCursor)
	b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row, col, FgGray+Dim, strings.Repeat(BoxH, width), Reset))
	b.WriteString(RestCursor)
	return b.String()
}

// DrawActivePaneIndicator highlights the border segments touching the
// active pane's rectangle (row, col, rows, cols) in the given
// totalRows x totalCols terminal.
//
// BUG FIX: this used to draw a solid bar glyph directly into the pane's own
// top-left content cells (the shell's actual output area), permanently
// corrupting whatever the pane had drawn there -- visible as a stray thick
// "cursor-like" mark that never went away after a split. Highlighting only
// ever touches border cells, which drawBorders() unconditionally repaints
// on every call, so switching the active pane always fully erases the
// previous highlight instead of leaving a stale mark behind.
func DrawActivePaneIndicator(row, col, rows, cols, totalRows, totalCols int) string {
	var b strings.Builder
	b.WriteString(SaveCursor)
	color := FgCyan + Bold

	if col > 1 {
		for i := 0; i < rows; i++ {
			b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row+i, col-1, color, BoxV, Reset))
		}
	}
	if col+cols <= totalCols {
		for i := 0; i < rows; i++ {
			b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row+i, col+cols, color, BoxV, Reset))
		}
	}
	if row > 1 {
		b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row-1, col, color, strings.Repeat(BoxH, cols), Reset))
	}
	if row+rows <= totalRows {
		b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row+rows, col, color, strings.Repeat(BoxH, cols), Reset))
	}

	b.WriteString(RestCursor)
	return b.String()
}
