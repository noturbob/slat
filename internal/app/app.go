package app

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/session"
	"github.com/noturbob/slat/internal/ui"
)

// App is the main application object tying everything together.
type App struct {
	cfg      *config.Config
	manager  *session.Manager
	handler  *input.Handler
	cols     int
	rows     int
	oldState *term.State
	quit     chan struct{}
	quitOnce sync.Once
	showHelp bool
	mu       sync.Mutex
}

// New creates a new App from the user config.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	mgr := session.NewManager(cfg.Shell)
	handler := input.NewHandler(cfg.Prefix, cfg.Keybinds)

	return &App{
		cfg:     cfg,
		manager: mgr,
		handler: handler,
		quit:    make(chan struct{}),
	}, nil
}

// Run starts the application.
func (a *App) Run() error {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to enter raw mode: %w", err)
	}
	a.oldState = oldState
	defer a.cleanup()

	a.cols, a.rows, err = term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	paneRows, paneCols := a.paneArea()
	if err := a.manager.Init(uint16(paneRows), uint16(paneCols)); err != nil {
		return fmt.Errorf("failed to init session: %w", err)
	}

	a.applyLayout()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-sigCh:
				a.handleResize()
			case <-a.quit:
				return
			}
		}
	}()

	termCh := make(chan os.Signal, 1)
	signal.Notify(termCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-termCh:
			a.doQuit()
		case <-a.quit:
		}
	}()

	os.Stdout.WriteString(ui.Clear)
	go a.forwardPaneOutput()
	go a.renderLoop()
	a.readInput()
	return nil
}

func (a *App) paneArea() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := a.rows - 1
	if rows < 1 {
		rows = 1
	}
	cols := a.cols
	if cols < 1 {
		cols = 1
	}
	return rows, cols
}

func (a *App) handleResize() {
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	a.mu.Lock()
	a.cols = cols
	a.rows = rows
	a.mu.Unlock()
	a.applyLayout()
	os.Stdout.WriteString(ui.Clear)
}

func (a *App) forwardPaneOutput() {
	for {
		select {
		case <-a.quit:
			return
		default:
		}

		panes := a.manager.GetAllPanesInActiveTab()
		if len(panes) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		didWork := false
		for _, p := range panes {
			if p.Dead() {
				continue
			}
			select {
			case data := <-p.Output:
				if len(data) > 0 {
					a.mu.Lock()
					row := p.Row
					col := p.Col
					a.mu.Unlock()
					if row == 0 {
						row = 1
					}
					if col == 0 {
						col = 1
					}
					activePane := a.manager.GetActivePane()
					if p == activePane || len(panes) == 1 {
						os.Stdout.WriteString(ui.MoveCursorToPane(row, col))
						os.Stdout.Write(data)
					} else {
						os.Stdout.WriteString(fmt.Sprintf("\x1b7\x1b[%d;%dH", row, col))
						os.Stdout.Write(data)
						os.Stdout.WriteString("\x1b8")
					}
					didWork = true
				}
			default:
			}
		}
		if !didWork {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (a *App) renderLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-ticker.C:
			a.render()
		}
	}
}

func (a *App) render() {
	a.mu.Lock()
	cols := a.cols
	rows := a.rows
	a.mu.Unlock()

	if shouldQuit := a.manager.CleanupDeadPanes(); shouldQuit {
		a.doQuit()
		return
	}

	mode := "NORMAL"
	if a.showHelp {
		mode = "HELP"
	}

	if a.cfg.StatusBar {
		statusBar := ui.DrawStatusBar(cols, rows, mode, a.manager, a.handler.IsPrefixActive())
		os.Stdout.WriteString(statusBar)
	}

	panes := a.manager.GetAllPanesInActiveTab()
	activePane := a.manager.GetActivePane()
	if len(panes) > 1 {
		for _, p := range panes {
			isActive := (p == activePane)
			indicator := ui.DrawActivePaneIndicator(p.Row, p.Col, isActive)
			if indicator != "" {
				os.Stdout.WriteString(indicator)
			}
		}
	}

	if a.showHelp {
		a.renderHelp(cols, rows)
	}
}

func (a *App) renderHelp(cols, rows int) {
	helpLines := []string{
		"\u256d\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500 SLAT HELP \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u256e",
		"\u2502                                  \u2502",
		fmt.Sprintf("\u2502  Prefix: %-24s \u2502", a.cfg.Prefix),
		"\u2502                                  \u2502",
		"\u2502  After prefix:                   \u2502",
		"\u2502    v  Split vertical              \u2502",
		"\u2502    h  Split horizontal            \u2502",
		"\u2502    o  Next pane                   \u2502",
		"\u2502    O  Previous pane               \u2502",
		"\u2502    x  Close pane                  \u2502",
		"\u2502    c  New tab                     \u2502",
		"\u2502    n  Next tab                    \u2502",
		"\u2502    p  Previous tab                \u2502",
		"\u2502    W  New workspace               \u2502",
		"\u2502    w  Next workspace              \u2502",
		"\u2502    z  Zoom pane                   \u2502",
		"\u2502    q  Quit                        \u2502",
		"\u2502    ?  This help                   \u2502",
		"\u2502                                  \u2502",
		"\u2502  Press any key to close           \u2502",
		"\u2570\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u256f",
	}
	startRow := (rows - len(helpLines)) / 2
	startCol := (cols - 36) / 2
	if startRow < 1 {
		startRow = 1
	}
	if startCol < 1 {
		startCol = 1
	}
	for i, line := range helpLines {
		os.Stdout.WriteString(fmt.Sprintf("\x1b[%d;%dH%s%s%s%s", startRow+i, startCol, ui.BgDark, ui.FgCyan, line, ui.Reset))
	}
}

func (a *App) readInput() {
	buf := make([]byte, 1024)
	for {
		select {
		case <-a.quit:
			return
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}

		for i := 0; i < n; i++ {
			b := buf[i]

			if a.showHelp {
				a.showHelp = false
				os.Stdout.WriteString(ui.Clear)
				a.render()
				continue
			}

			action := a.handler.ProcessByte(b)

			switch action {
			case input.ActionForwardInput:
				activePane := a.manager.GetActivePane()
				if activePane != nil {
					if i == 0 && n > 1 && !a.handler.IsPrefixActive() {
						activePane.Write(buf[:n])
						i = n
					} else {
						activePane.Write([]byte{b})
					}
				}

			case input.ActionSplitVertical:
				pRows, pCols := a.paneArea()
				a.manager.SplitPane(pane.SplitVertical, uint16(pRows), uint16(pCols))
				a.applyLayout()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionSplitHorizontal:
				pRows, pCols := a.paneArea()
				a.manager.SplitPane(pane.SplitHorizontal, uint16(pRows), uint16(pCols))
				a.applyLayout()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionNextPane:
				a.manager.NextPane()

			case input.ActionPrevPane:
				a.manager.PrevPane()

			case input.ActionClosePane:
				if shouldQuit := a.manager.KillActivePane(); shouldQuit {
					a.doQuit()
					return
				}
				a.applyLayout()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionNewTab:
				pRows, pCols := a.paneArea()
				a.manager.CreateTab("Shell", uint16(pRows), uint16(pCols))
				os.Stdout.WriteString(ui.Clear)

			case input.ActionNextTab:
				a.manager.NextTab()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionPrevTab:
				a.manager.PrevTab()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionNewWorkspace:
				pRows, pCols := a.paneArea()
				wsCount := len(a.manager.Workspaces)
				a.manager.AddWorkspace(fmt.Sprintf("WS-%d", wsCount+1), uint16(pRows), uint16(pCols))
				os.Stdout.WriteString(ui.Clear)

			case input.ActionNextWorkspace:
				a.manager.NextWorkspace()
				os.Stdout.WriteString(ui.Clear)

			case input.ActionZoom:
				activeP := a.manager.GetActivePane()
				if activeP != nil {
					pRows, pCols := a.paneArea()
					activeP.Resize(uint16(pRows), uint16(pCols))
					activeP.SetPosition(1, 1)
					os.Stdout.WriteString(ui.Clear)
				}

			case input.ActionShowHelp:
				a.showHelp = true

			case input.ActionQuit:
				a.doQuit()
				return

			case input.ActionNone:
				// Prefix key was pressed
			}
			a.render()
		}
	}
}

func (a *App) applyLayout() {
	pRows, pCols := a.paneArea()
	a.manager.ApplyLayout(layout.Rect{
		Row:  1,
		Col:  1,
		Rows: pRows,
		Cols: pCols,
	})
	a.drawBorders()
}

func (a *App) drawBorders() {
	panes := a.manager.GetAllPanesInActiveTab()
	if len(panes) <= 1 {
		return
	}
	for _, p := range panes {
		col := p.Col
		if col > 1 {
			os.Stdout.WriteString(ui.DrawVerticalBorder(p.Row, col-1, int(p.Rows)))
		}
		row := p.Row
		if row > 1 {
			os.Stdout.WriteString(ui.DrawHorizontalBorder(row-1, p.Col, int(p.Cols)))
		}
	}
}

func (a *App) doQuit() {
	a.quitOnce.Do(func() {
		close(a.quit)
	})
}

func (a *App) cleanup() {
	os.Stdout.WriteString(ui.ShowCursor)
	os.Stdout.WriteString(ui.Clear)
	os.Stdout.WriteString("\x1b[0m")

	if a.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), a.oldState)
	}

	fmt.Println("slat: session ended")
}
