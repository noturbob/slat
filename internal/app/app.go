package app

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

// App is the slat session engine.
//
// App owns the workspace/tab/pane state and renders ANSI output to the
// writer configured with SetOutput. Input is supplied by the host through
// FeedInput, and terminal resize events are supplied through HandleResize.
//
// The App itself does not own a terminal and does not read stdin directly.
// This allows the same session to survive client detach/reconnect events.
type App struct {
	cfg     *config.Config
	manager *session.Manager
	handler *input.Handler

	cols int
	rows int

	quit     chan struct{}
	quitOnce sync.Once

	// detachCh is signalled when the attached client should disconnect,
	// while the underlying slat session remains alive.
	detachCh chan struct{}

	// showHelp controls whether the help overlay is currently visible.
	showHelp atomic.Bool

	// zoomedPane tracks the pane currently shown fullscreen, if any.
	zoomedPane *pane.Pane

	// stdout is the output writer configured by the host.
	//
	// All terminal output must go through outMu.
	stdout *bufio.Writer
	outMu  sync.Mutex

	// sizeMu protects cols/rows.
	sizeMu sync.RWMutex

	// Prompt state.
	promptMode  string
	promptBuf   []byte
	promptLabel string
}

// New creates a new App from the given config.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	mgr := session.NewManager(cfg.Shell)
	handler := input.NewHandler(cfg.Prefix, cfg.Keybinds)

	return &App{
		cfg:      cfg,
		manager:  mgr,
		handler:  handler,
		quit:     make(chan struct{}),
		detachCh: make(chan struct{}, 1),
	}, nil
}

// SetOutput configures where rendered ANSI output is written.
//
// The host should call this exactly once before Start().
func (a *App) SetOutput(w io.Writer) {
	a.outMu.Lock()
	defer a.outMu.Unlock()

	a.stdout = bufio.NewWriterSize(w, 32768)
}

// Done reports when the session has fully ended.
//
// This fires when the user quits or when the last pane dies on its own.
// The host should shut down the session when this channel fires.
func (a *App) Done() <-chan struct{} {
	return a.quit
}

// DetachRequested reports when the user requested a detach.
//
// A detach only disconnects the current client. The session itself continues
// running and can later be attached again.
func (a *App) DetachRequested() <-chan struct{} {
	return a.detachCh
}

// Shutdown terminates every pane's shell process.
//
// Call this after Done() fires, or when the host is forcibly shutting down
// the session.
func (a *App) Shutdown() {
	a.manager.Shutdown()
}

// write atomically writes a string to the configured output.
func (a *App) write(s string) {
	a.outMu.Lock()
	defer a.outMu.Unlock()

	if a.stdout == nil {
		return
	}

	a.stdout.WriteString(s)
	a.stdout.Flush()
}

// writeBytes atomically writes raw bytes to the configured output.
func (a *App) writeBytes(b []byte) {
	a.outMu.Lock()
	defer a.outMu.Unlock()

	if a.stdout == nil {
		return
	}

	a.stdout.Write(b)
	a.stdout.Flush()
}

// getSize returns the current terminal dimensions.
func (a *App) getSize() (cols, rows int) {
	a.sizeMu.RLock()
	cols = a.cols
	rows = a.rows
	a.sizeMu.RUnlock()

	return
}

// paneArea returns the usable area for panes.
//
// If the status bar is enabled, the last terminal row is reserved for it.
func (a *App) paneArea() (int, int) {
	cols, rows := a.getSize()

	pRows := rows
	if a.cfg.StatusBar {
		pRows = rows - 1
	}

	if pRows < 1 {
		pRows = 1
	}

	if cols < 1 {
		cols = 1
	}

	return pRows, cols
}

// Start initializes the session and starts the background rendering loops.
//
// Start does not block.
func (a *App) Start(cols, rows int) error {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}

	a.sizeMu.Lock()
	a.cols = cols
	a.rows = rows
	a.sizeMu.Unlock()

	a.outMu.Lock()
	outputConfigured := a.stdout != nil
	a.outMu.Unlock()

	if !outputConfigured {
		return fmt.Errorf("output writer not configured; call SetOutput before Start")
	}

	paneRows, paneCols := a.paneArea()

	if err := a.manager.Init(uint16(paneRows), uint16(paneCols)); err != nil {
		return fmt.Errorf("failed to init session: %w", err)
	}

	a.applyLayout()
	a.setupScrollRegion()

	a.write(ui.Clear)
	a.drawBorders()
	a.renderStatusBar()

	go a.forwardPaneOutput()
	go a.statusLoop()

	return nil
}

// setupScrollRegion configures the terminal scroll region according to the
// currently configured pane area.
func (a *App) setupScrollRegion() {
	paneRows, _ := a.paneArea()

	a.write(
		ui.SetScrollRegion(1, paneRows) +
			"\033[1;1H",
	)
}

// HandleResize applies a new terminal size reported by the attached client.
//
// The caller is responsible for obtaining the dimensions from its own
// terminal/client connection.
func (a *App) HandleResize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}

	a.sizeMu.Lock()
	a.cols = cols
	a.rows = rows
	a.sizeMu.Unlock()

	if a.zoomedPane != nil {
		pRows, pCols := a.paneArea()

		a.zoomedPane.Resize(
			uint16(pRows),
			uint16(pCols),
		)
		a.zoomedPane.SetPosition(1, 1)
	} else {
		a.applyLayout()
	}

	a.setupScrollRegion()
	a.fullRedraw()

	if a.showHelp.Load() {
		a.renderHelp()
	}

	if a.promptMode != "" {
		a.drawPrompt()
	}
}

// fullRedraw clears the screen and redraws the application UI.
func (a *App) fullRedraw() {
	a.write(ui.Clear)
	a.drawBorders()
	a.renderStatusBar()
}

// visiblePanes returns the panes that should currently be rendered.
//
// When zoomed, only the zoomed pane is visible. Otherwise all panes in the
// active tab are visible.
func (a *App) visiblePanes() []*pane.Pane {
	if a.zoomedPane != nil {
		return []*pane.Pane{a.zoomedPane}
	}

	return a.manager.GetAllPanesInActiveTab()
}

// forwardPaneOutput reads PTY output from all visible panes and forwards it
// to the configured output writer.
//
// This is the only goroutine responsible for forwarding pane content.
func (a *App) forwardPaneOutput() {
	for {
		select {
		case <-a.quit:
			return
		default:
		}

		panes := a.visiblePanes()

		if len(panes) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		activePane := a.manager.GetActivePane()
		singlePane := len(panes) == 1
		didWork := false

		for _, p := range panes {
			if p.Dead() {
				continue
			}

			select {
			case data := <-p.Output:
				if len(data) == 0 {
					continue
				}

				didWork = true

				pRow, pCol, pRows, _ := p.Rect()

				if singlePane || p == activePane {
					if singlePane {
						a.writeBytes(data)
					} else {
						a.outMu.Lock()

						if a.stdout != nil {
							a.stdout.WriteString(ui.SaveCursor)
							a.stdout.WriteString(
								ui.SetScrollRegion(
									pRow,
									pRow+int(pRows)-1,
								),
							)
							a.stdout.WriteString(ui.RestCursor)
							a.stdout.Write(data)

							paneRows, _ := a.paneArea()

							a.stdout.WriteString(
								ui.SetScrollRegion(1, paneRows),
							)

							a.stdout.Flush()
						}

						a.outMu.Unlock()
					}
				} else {
					a.outMu.Lock()

					if a.stdout != nil {
						a.stdout.WriteString(ui.SaveCursor)

						a.stdout.WriteString(
							ui.SetScrollRegion(
								pRow,
								pRow+int(pRows)-1,
							),
						)

						a.stdout.WriteString(
							fmt.Sprintf(
								"\033[%d;%dH",
								pRow,
								pCol,
							),
						)

						a.stdout.Write(data)

						paneRows, _ := a.paneArea()

						a.stdout.WriteString(
							ui.SetScrollRegion(1, paneRows),
						)

						a.stdout.WriteString(ui.RestCursor)

						a.stdout.Flush()
					}

					a.outMu.Unlock()
				}

			default:
			}
		}

		if !didWork {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// statusLoop periodically updates the status bar and cleans up dead panes.
func (a *App) statusLoop() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-a.quit:
			return

		case <-ticker.C:
			if shouldQuit := a.manager.CleanupDeadPanes(); shouldQuit {
				a.doQuit()
				return
			}

			// If the zoomed pane dies, remove the zoom state before the
			// manager's layout tree is rendered again.
			if a.zoomedPane != nil && a.zoomedPane.Dead() {
				a.zoomedPane = nil
				a.applyLayout()
				a.setupScrollRegion()
				a.fullRedraw()
			}

			a.renderStatusBar()
		}
	}
}

// renderStatusBar draws the status bar without disturbing the PTY cursor.
func (a *App) renderStatusBar() {
	if !a.cfg.StatusBar {
		return
	}

	cols, rows := a.getSize()

	mode := "NORMAL"

	if a.showHelp.Load() {
		mode = "HELP"
	}

	bar := ui.DrawStatusBar(
		cols,
		rows,
		mode,
		a.manager,
		a.handler.IsPrefixActive(),
	)

	if bar != "" {
		a.write(bar)
	}
}

// renderHelp renders the keybind help overlay.
func (a *App) renderHelp() {
	cols, rows := a.getSize()

	w := 52

	helpLines := []string{
		"\u256d" + rep("\u2500", w-2) + "\u256e",
		"\u2502" + centerPad("SLAT \u2500 Keybind Reference", w-2) + "\u2502",
		"\u2502" + rep(" ", w-2) + "\u2502",

		"\u251c" + rep("\u2500", w-2) + "\u2524",
		"\u2502" + centerPad(
			"\033[1m\033[36m\u2500\u2500 Panes \u2500\u2500\033[0m\033[36m",
			w-2+14,
		) + "\u2502",

		"\u2502" + rep(" ", w-2) + "\u2502",

		fmtKey(w, "v", "Split pane vertically"),
		fmtKey(w, "h", "Split pane horizontally"),
		fmtKey(w, "o", "Focus next pane"),
		fmtKey(w, "O", "Focus previous pane"),
		fmtKey(w, "\u2191 k", "Focus pane above"),
		fmtKey(w, "\u2193 j", "Focus pane below"),
		fmtKey(w, "\u2190 H", "Focus pane left"),
		fmtKey(w, "\u2192 L", "Focus pane right"),
		fmtKey(w, "s", "Swap pane with next"),
		fmtKey(w, "+", "Grow pane"),
		fmtKey(w, "-", "Shrink pane"),
		fmtKey(w, "=", "Equalize pane sizes"),
		fmtKey(w, "z", "Toggle zoom pane"),
		fmtKey(w, "x", "Close pane"),

		"\u2502" + rep(" ", w-2) + "\u2502",

		"\u251c" + rep("\u2500", w-2) + "\u2524",
		"\u2502" + centerPad(
			"\033[1m\033[36m\u2500\u2500 Tabs \u2500\u2500\033[0m\033[36m",
			w-2+14,
		) + "\u2502",

		"\u2502" + rep(" ", w-2) + "\u2502",

		fmtKey(w, "c", "Create new tab"),
		fmtKey(w, "n", "Next tab"),
		fmtKey(w, "p", "Previous tab"),
		fmtKey(w, "1-9", "Jump to tab #"),
		fmtKey(w, ",", "Rename current tab"),
		fmtKey(w, "X", "Close entire tab"),

		"\u2502" + rep(" ", w-2) + "\u2502",

		"\u251c" + rep("\u2500", w-2) + "\u2524",
		"\u2502" + centerPad(
			"\033[1m\033[36m\u2500\u2500 Workspaces \u2500\u2500\033[0m\033[36m",
			w-2+14,
		) + "\u2502",

		"\u2502" + rep(" ", w-2) + "\u2502",

		fmtKey(w, "W", "Create new workspace"),
		fmtKey(w, "w", "Next workspace"),
		fmtKey(w, "P", "Previous workspace"),
		fmtKey(w, "$", "Rename workspace"),

		"\u2502" + rep(" ", w-2) + "\u2502",

		"\u251c" + rep("\u2500", w-2) + "\u2524",
		"\u2502" + centerPad(
			"\033[1m\033[36m\u2500\u2500 Session \u2500\u2500\033[0m\033[36m",
			w-2+14,
		) + "\u2502",

		"\u2502" + rep(" ", w-2) + "\u2502",

		fmtKey(w, "?", "Show this help"),
		fmtKey(w, "d", "Detach (session keeps running)"),
		fmtKey(w, "q", "Quit slat (ends the session)"),

		fmt.Sprintf(
			"\u2502  Prefix: %-*s\u2502",
			w-13,
			a.cfg.Prefix,
		),

		"\u2502" + rep(" ", w-2) + "\u2502",

		"\u2502" + centerPad(
			"Press any key to close",
			w-2,
		) + "\u2502",

		"\u2570" + rep("\u2500", w-2) + "\u256f",
	}

	startRow := (rows - len(helpLines)) / 2
	startCol := (cols - w) / 2

	if startRow < 1 {
		startRow = 1
	}

	if startCol < 1 {
		startCol = 1
	}

	a.outMu.Lock()
	defer a.outMu.Unlock()

	if a.stdout == nil {
		return
	}

	a.stdout.WriteString(ui.SaveCursor)
	a.stdout.WriteString(ui.HideCursor)

	for i, line := range helpLines {
		a.stdout.WriteString(
			fmt.Sprintf(
				"\033[%d;%dH%s%s%s%s",
				startRow+i,
				startCol,
				ui.BgDark,
				ui.FgCyan,
				line,
				ui.Reset,
			),
		)
	}

	a.stdout.WriteString(ui.RestCursor)
	a.stdout.WriteString(ui.ShowCursor)
	a.stdout.Flush()
}

// fmtKey formats one keybind line for the help overlay.
func fmtKey(w int, key, desc string) string {
	inner := fmt.Sprintf(
		"  \033[1m\033[97m%-5s\033[0m%s\033[36m \u00b7  %s",
		key,
		ui.BgDark,
		desc,
	)

	visLen := 5 + 4 + len(desc) + 2

	pad := w - 2 - visLen

	if pad < 0 {
		pad = 0
	}

	return "\u2502" + inner + rep(" ", pad) + "\u2502"
}

func rep(s string, n int) string {
	if n <= 0 {
		return ""
	}

	var sb strings.Builder

	for i := 0; i < n; i++ {
		sb.WriteString(s)
	}

	return sb.String()
}

func centerPad(s string, w int) string {
	vis := 0
	inEsc := false

	for _, c := range s {
		if c == '\033' {
			inEsc = true
			continue
		}

		if inEsc {
			if (c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') {
				inEsc = false
			}

			continue
		}

		vis++
	}

	total := w - vis

	if total <= 0 {
		return s
	}

	left := total / 2
	right := total - left

	return rep(" ", left) + s + rep(" ", right)
}

// ─── Zoom ───────────────────────────────────────────────────────────────

func (a *App) toggleZoom() {
	if a.zoomedPane != nil {
		a.clearZoom()
		a.applyLayout()
		a.setupScrollRegion()
		a.fullRedraw()
		return
	}

	p := a.manager.GetActivePane()

	if p == nil {
		return
	}

	pRows, pCols := a.paneArea()

	p.Resize(
		uint16(pRows),
		uint16(pCols),
	)

	p.SetPosition(1, 1)
	p.Zoomed = true

	a.zoomedPane = p

	a.setupScrollRegion()
	a.fullRedraw()
}

func (a *App) clearZoom() {
	if a.zoomedPane != nil {
		a.zoomedPane.Zoomed = false
		a.zoomedPane = nil
	}
}

// ─── Prompt mode ────────────────────────────────────────────────────────

func (a *App) startPrompt(purpose, label string) {
	a.promptMode = purpose
	a.promptLabel = label
	a.promptBuf = nil

	a.drawPrompt()
}

func (a *App) drawPrompt() {
	cols, rows := a.getSize()

	text := string(a.promptBuf)

	line := fmt.Sprintf(
		" %s: %s\u2588 ",
		a.promptLabel,
		text,
	)

	visLen := len(a.promptLabel) +
		2 +
		len(text) +
		3

	a.outMu.Lock()
	defer a.outMu.Unlock()

	if a.stdout == nil {
		return
	}

	a.stdout.WriteString(ui.SaveCursor)

	a.stdout.WriteString(
		fmt.Sprintf(
			"\033[%d;1H\033[2K",
			rows,
		),
	)

	a.stdout.WriteString(ui.BgGreen)
	a.stdout.WriteString(ui.FgBlack)
	a.stdout.WriteString(ui.Bold)

	a.stdout.WriteString(line)

	if cols-visLen > 0 {
		a.stdout.WriteString(
			rep(" ", cols-visLen),
		)
	}

	a.stdout.WriteString(ui.Reset)
	a.stdout.WriteString(ui.RestCursor)

	a.stdout.Flush()
}

func (a *App) finishPrompt() string {
	result := string(a.promptBuf)

	a.promptMode = ""
	a.promptBuf = nil
	a.promptLabel = ""

	a.renderStatusBar()

	return result
}

func (a *App) cancelPrompt() {
	a.promptMode = ""
	a.promptBuf = nil
	a.promptLabel = ""

	a.renderStatusBar()
}

// ─── Input ──────────────────────────────────────────────────────────────

// FeedInput processes raw input bytes received from the attached client.
//
// The daemon should call this for every Input frame received from a client.
// It supports normal keystrokes, paste, prefix commands, prompts and all
// existing pane/tab/workspace operations.
func (a *App) FeedInput(buf []byte) {
	n := len(buf)

	for i := 0; i < n; i++ {
		b := buf[i]

		// ── Prompt mode ────────────────────────────────────────────────

		if a.promptMode != "" {
			switch b {
			case 13: // Enter
				purpose := a.promptMode
				text := a.finishPrompt()

				if text != "" {
					switch purpose {
					case "rename-tab":
						a.manager.RenameTab(text)

					case "rename-workspace":
						a.manager.RenameWorkspace(text)
					}
				}

				a.renderStatusBar()

			case 27: // Escape
				a.cancelPrompt()

			case 127, 8: // Backspace
				if len(a.promptBuf) > 0 {
					a.promptBuf =
						a.promptBuf[:len(a.promptBuf)-1]

					a.drawPrompt()
				}

			default:
				if b >= 32 && b < 127 {
					a.promptBuf =
						append(a.promptBuf, b)

					a.drawPrompt()
				}
			}

			continue
		}

		// ── Help overlay ──────────────────────────────────────────────

		if a.showHelp.Load() {
			a.showHelp.Store(false)
			a.fullRedraw()
			continue
		}

		action := a.handler.ProcessByte(b)

		// Any layout-affecting action exits zoom mode first.
		if action != input.ActionForwardInput &&
			action != input.ActionZoom &&
			action != input.ActionNone &&
			action != input.ActionSendPrefix {
			a.clearZoom()
		}

		switch action {

		// ── Normal shell input ────────────────────────────────────────

		case input.ActionForwardInput:
			activePane := a.manager.GetActivePane()

			if activePane == nil {
				continue
			}

			// Bulk-forward normal input until the next prefix byte.
			//
			// This preserves prefix detection even when multiple bytes
			// arrive together in a single client frame.
			if i == 0 {
				j := i
				prefixByte := a.handler.PrefixByte()

				for j < n && buf[j] != prefixByte {
					j++
				}

				if j > i {
					activePane.Write(buf[i:j])
					i = j - 1
					continue
				}
			}

			activePane.Write([]byte{b})

		// ── Pane commands ─────────────────────────────────────────────

		case input.ActionSplitVertical:
			pRows, pCols := a.paneArea()

			a.manager.SplitPane(
				pane.SplitVertical,
				uint16(pRows),
				uint16(pCols),
			)

			a.applyLayout()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionSplitHorizontal:
			pRows, pCols := a.paneArea()

			a.manager.SplitPane(
				pane.SplitHorizontal,
				uint16(pRows),
				uint16(pCols),
			)

			a.applyLayout()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionNextPane:
			a.manager.NextPane()
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionPrevPane:
			a.manager.PrevPane()
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionSelectPaneUp:
			a.manager.SelectPaneInDirection("up")
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionSelectPaneDown:
			a.manager.SelectPaneInDirection("down")
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionSelectPaneLeft:
			a.manager.SelectPaneInDirection("left")
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionSelectPaneRight:
			a.manager.SelectPaneInDirection("right")
			a.drawBorders()
			a.renderStatusBar()

		case input.ActionSwapPane:
			a.manager.SwapPanes()
			a.applyLayout()
			a.fullRedraw()

		case input.ActionResizeGrow:
			a.manager.ResizeRatio(0.05)
			a.applyLayout()
			a.fullRedraw()

		case input.ActionResizeShrink:
			a.manager.ResizeRatio(-0.05)
			a.applyLayout()
			a.fullRedraw()

		case input.ActionEqualizeLayout:
			a.manager.EqualizeLayout()
			a.applyLayout()
			a.fullRedraw()

		case input.ActionZoom:
			a.toggleZoom()

		case input.ActionClosePane:
			if shouldQuit := a.manager.KillActivePane(); shouldQuit {
				a.doQuit()
				return
			}

			a.applyLayout()
			a.setupScrollRegion()
			a.fullRedraw()

		// ── Tab commands ──────────────────────────────────────────────

		case input.ActionNewTab:
			pRows, pCols := a.paneArea()

			a.manager.CreateTab(
				"Shell",
				uint16(pRows),
				uint16(pCols),
			)

			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionNextTab:
			a.manager.NextTab()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionPrevTab:
			a.manager.PrevTab()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionGoToTab1,
			input.ActionGoToTab2,
			input.ActionGoToTab3,
			input.ActionGoToTab4,
			input.ActionGoToTab5,
			input.ActionGoToTab6,
			input.ActionGoToTab7,
			input.ActionGoToTab8,
			input.ActionGoToTab9:

			idx := int(action - input.ActionGoToTab1)

			a.manager.GoToTab(idx)

			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionRenameTab:
			a.startPrompt(
				"rename-tab",
				"Rename tab",
			)

		case input.ActionCloseTab:
			if shouldQuit := a.manager.CloseTab(); shouldQuit {
				a.doQuit()
				return
			}

			a.setupScrollRegion()
			a.fullRedraw()

		// ── Workspace commands ────────────────────────────────────────

		case input.ActionNewWorkspace:
			pRows, pCols := a.paneArea()

			wsCount := len(a.manager.Workspaces)

			a.manager.AddWorkspace(
				fmt.Sprintf(
					"WS-%d",
					wsCount+1,
				),
				uint16(pRows),
				uint16(pCols),
			)

			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionNextWorkspace:
			a.manager.NextWorkspace()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionPrevWorkspace:
			a.manager.PrevWorkspace()
			a.setupScrollRegion()
			a.fullRedraw()

		case input.ActionRenameWorkspace:
			a.startPrompt(
				"rename-workspace",
				"Rename workspace",
			)

		// ── Session commands ──────────────────────────────────────────

		case input.ActionShowHelp:
			a.showHelp.Store(true)
			a.renderHelp()

		case input.ActionSendPrefix:
			activePane := a.manager.GetActivePane()

			if activePane != nil {
				activePane.Write([]byte{b})
			}

		case input.ActionDetach:
			// Do NOT close a.quit here.
			//
			// Detach means:
			//   client -> disconnect
			//   session -> continues running
			select {
			case a.detachCh <- struct{}{}:
			default:
				// Already signalled.
			}

			return

		case input.ActionQuit:
			a.doQuit()
			return

		case input.ActionNone:
			a.renderStatusBar()
		}
	}
}

// applyLayout applies the current layout to the usable pane area.
func (a *App) applyLayout() {
	pRows, pCols := a.paneArea()

	a.manager.ApplyLayout(
		layout.Rect{
			Row:  1,
			Col:  1,
			Rows: pRows,
			Cols: pCols,
		},
	)
}

// drawBorders renders pane borders and the active pane indicator.
func (a *App) drawBorders() {
	panes := a.visiblePanes()

	if len(panes) <= 1 {
		return
	}

	a.outMu.Lock()
	defer a.outMu.Unlock()

	if a.stdout == nil {
		return
	}

	a.stdout.WriteString(ui.SaveCursor)

	for _, p := range panes {
		row, col, rows, cols := p.Rect()

		if col > 1 {
			a.stdout.WriteString(
				ui.DrawVerticalBorder(
					row,
					col-1,
					int(rows),
				),
			)
		}

		if row > 1 {
			a.stdout.WriteString(
				ui.DrawHorizontalBorder(
					row-1,
					col,
					int(cols),
				),
			)
		}
	}

	activePane := a.manager.GetActivePane()

	for _, p := range panes {
		if p == activePane {
			row, col, rows, cols := p.Rect()

			a.stdout.WriteString(
				ui.DrawActivePaneIndicator(
					row,
					col,
					int(rows),
					int(cols),
					true,
				),
			)
		}
	}

	a.stdout.WriteString(ui.RestCursor)
	a.stdout.Flush()
}

// doQuit marks the session as terminated.
//
// Shutdown of pane processes is intentionally separate and is performed
// through Shutdown() by the host.
func (a *App) doQuit() {
	a.quitOnce.Do(func() {
		close(a.quit)
	})
}