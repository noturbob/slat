package ui

import (
	"fmt"
	"strings"

	"github.com/noturbob/slat/internal/session"
)

// ANSI escape codes.
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
	Clear      = "\033[2J\033[H"

	// Box drawing characters.
	BoxH = "\u2500"
	BoxV = "\u2502"
)

// GetBanner returns the startup banner.
func GetBanner() string {
	return FgCyan + Bold + `
   _____ __      ___  _______
  / ___// /     /   |/_  __/
  \__ \/ /     / /| | / /   
 ___/ / /___  / ___ |/ /    
/____/_____/ /_/  |_/_/     
` + Reset + Dim + `
  Terminal Multiplexer v0.1.0
  Press your prefix key + ? for help
` + Reset
}

// DrawStatusBar renders the status bar at the bottom of the terminal.
func DrawStatusBar(cols, rows int, mode string, manager *session.Manager, prefixActive bool) string {
	if rows < 2 || cols < 2 {
		return ""
	}
	ws := manager.GetCurrentWorkspace()
	if ws == nil {
		return ""
	}

	var bar strings.Builder

	// Move to last line
	bar.WriteString(fmt.Sprintf("\033[%d;1H", rows))

	// Background fill
	bar.WriteString(BgDark)
	bar.WriteString(strings.Repeat(" ", cols))
	bar.WriteString(fmt.Sprintf("\033[%d;1H", rows))

	// Left side: workspace indicator
	bar.WriteString(fmt.Sprintf(" %s[%s]%s ", FgGreen+Bold, ws.Name, Reset+BgDark))

	// Separator
	bar.WriteString(FgGray + "\u2502" + Reset + BgDark + " ")

	// Tabs
	for i, tab := range ws.Tabs {
		name := fmt.Sprintf(" %d:%s ", i+1, tab.Name)
		if i == ws.ActiveTabIdx {
			bar.WriteString(BgBlue + FgWhite + Bold + name + Reset + BgDark)
		} else {
			bar.WriteString(BgDarkAlt + FgGray + name + Reset + BgDark)
		}
		bar.WriteString(" ")
	}

	// Right side: mode + pane info
	activePane := manager.GetActivePane()
	paneID := 0
	if activePane != nil {
		paneID = activePane.ID
	}

	modeStr := ""
	if prefixActive {
		modeStr = BgGreen + FgBlack + Bold + " PREFIX " + Reset + BgDark + " "
	} else if mode != "" && mode != "NORMAL" {
		modeStr = BgGreen + FgBlack + Bold + fmt.Sprintf(" %s ", mode) + Reset + BgDark + " "
	}

	rightSide := fmt.Sprintf("%s%sPane:%d%s ", modeStr, FgGray, paneID, Reset+BgDark)

	// Position the right side near the right edge
	rightPos := cols - 20
	if rightPos < 1 {
		rightPos = 1
	}
	bar.WriteString(fmt.Sprintf("\033[%d;%dH", rows, rightPos))
	bar.WriteString(rightSide)

	bar.WriteString(Reset)
	return bar.String()
}

// DrawActivePaneIndicator draws a marker next to the active pane.
func DrawActivePaneIndicator(row, col int, active bool) string {
	if !active {
		return ""
	}
	return fmt.Sprintf("\033[%d;%dH%s\u258e%s", row, col, FgCyan+Bold, Reset)
}

// MoveCursorToPane returns an escape sequence to move the cursor to a pane's position.
func MoveCursorToPane(row, col int) string {
	return fmt.Sprintf("\033[%d;%dH", row, col)
}

// DrawVerticalBorder draws a vertical border line.
func DrawVerticalBorder(row, col, height int) string {
	var b strings.Builder
	for i := 0; i < height; i++ {
		b.WriteString(fmt.Sprintf("\033[%d;%dH%s%s%s", row+i, col, FgGray, BoxV, Reset))
	}
	return b.String()
}

// DrawHorizontalBorder draws a horizontal border line.
func DrawHorizontalBorder(row, col, width int) string {
	return fmt.Sprintf("\033[%d;%dH%s%s%s", row, col, FgGray, strings.Repeat(BoxH, width), Reset)
}
