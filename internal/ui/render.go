package ui

import (
	"fmt"
	"strings"

	"github.com/noturbob/slat/internal/session"
)

const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	BgBlue     = "\033[44m"
	BgGreen    = "\033[42m"
	BgGray     = "\033[100m"
	BgDark     = "\033[40m"
	FgWhite    = "\033[37m"
	FgBlack    = "\033[30m"
	HideCursor = "\033[?25l"
	ShowCursor = "\033[?25h"
	Clear      = "\033[2J\033[H"
)

func GetBanner() string {
	return `
   _____ __      ___  _______
  / ___// /     /   |/_  __/
  \__ \/ /     / /| | / /   
 ___/ / /___  / ___ |/ /    
/____/_____/ /_/  |_/_/     
                            
`
}

func DrawStatusBar(cols, rows int, mode string, manager *session.Manager) string {
	if rows < 2 || cols < 2 { return "" }
	ws := manager.GetCurrentWorkspace()
	if ws == nil { return "" }

	// Build Tabs
	var tabs strings.Builder
	tabs.WriteString(" ")
	for i, tab := range ws.Tabs {
		name := fmt.Sprintf(" %d:%s ", i+1, tab.Name)
		if i == ws.ActiveTabIdx {
			tabs.WriteString(BgBlue + FgWhite + Bold + name + Reset)
		} else {
			tabs.WriteString(BgGray + FgWhite + name + Reset)
		}
		tabs.WriteString(" ")
	}

	// Info
	activePane := manager.GetActivePane()
	pid := 0
	if activePane != nil { pid = activePane.ID }

	modeColor := BgGray
	if mode != "NORMAL" { modeColor = BgGreen + FgBlack }

	info := fmt.Sprintf("%s %s %s Pane: %d ", modeColor, mode, Reset, pid)

	moveBottom := fmt.Sprintf("\033[%d;1H", rows)
	
	// Clear line background
	bg := BgDark + strings.Repeat(" ", cols) + Reset
	
	// Assemble: Save Cursor -> Hide -> Move -> Draw BG -> Draw Tabs -> Draw Info -> Restore -> Show
	return fmt.Sprintf("\0337%s%s%s\r%s%s%s\0338",
		HideCursor,
		moveBottom,
		bg,
		tabs.String(),
		strings.Repeat(" ", 4),
		info,
	)
}