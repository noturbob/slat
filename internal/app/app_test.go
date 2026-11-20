//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/vt"
)

// terminal stands in for the user's real terminal: it interprets
// everything slat sends, so tests can assert on what the user would see.
type terminal struct {
	mu  sync.Mutex
	emu *vt.Terminal
	raw []byte // everything written, including sequences the screen swallows
}

func (t *terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.raw = append(t.raw, p...)
	return t.emu.Write(p)
}

// rawString is every byte slat has sent to the terminal.
func (t *terminal) rawString() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.raw)
}

func (t *terminal) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.emu.String()
}

// cell is what the user's terminal holds at (x, y), style included.
func (t *terminal) cell(x, y int) vt.Cell {
	t.mu.Lock()
	defer t.mu.Unlock()
	line := t.emu.Line(y)
	if x < 0 || x >= len(line) {
		return vt.Cell{}
	}
	return line[x]
}

const prefix = 0x13 // Ctrl-S

func start(t *testing.T, tweak ...func(*config.Config)) (*App, *terminal) {
	t.Helper()
	t.Setenv("PS1", "slat$ ")
	// Never the real one: a test must not read, write or delete the
	// session the user has running. A test that wants two runs to share a
	// session sets this itself, and keeps it.
	if os.Getenv("XDG_STATE_HOME") == "" {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
	}
	t.Setenv("ENV", "") // keep sh from sourcing rc files that reset PS1
	cfg := config.DefaultConfig()
	cfg.Shell = "/bin/sh"
	for _, f := range tweak {
		f(cfg)
	}
	term := &terminal{emu: vt.New(100, 30)}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.SetOutput(term)
	if err := a.Start(100, 30); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Shutdown)
	a.Attach(100, 30)
	waitFor(t, term, "slat$", 1)
	return a, term
}

// waitFor waits until s appears at least n times on the terminal screen.
func waitFor(t *testing.T, term *terminal, s string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(term.String(), s) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d× %q; screen:\n%s", n, s, term.String())
}

func waitGone(t *testing.T, term *terminal, s string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !strings.Contains(term.String(), s) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%q still on screen:\n%s", s, term.String())
}

// The reported bug: opening panes and closing them left the screen blank or
// wiped the shell prompt, because the screen was cleared and pane contents
// were never redrawn.
func TestSplitAndCloseKeepPaneContents(t *testing.T) {
	a, term := start(t)

	a.FeedInput([]byte("echo first-pane-output\r"))
	waitFor(t, term, "first-pane-output", 2) // command line + output

	for _, key := range []byte{'v', 'h'} {
		a.FeedInput([]byte{prefix, key})
		waitFor(t, term, "slat$", 3) // one more prompt, in the new pane
		a.FeedInput([]byte("echo in-new-pane\r"))
		waitFor(t, term, "in-new-pane", 2)

		a.FeedInput([]byte{prefix, 'x'})
		waitGone(t, term, "in-new-pane")
		// The original pane — its history and its prompt — is fully back.
		waitFor(t, term, "first-pane-output", 2)
		waitFor(t, term, "slat$", 2)
		waitFor(t, term, "pane 1/1", 1)
	}

	// Typing still reaches the surviving pane.
	a.FeedInput([]byte("echo still-alive\r"))
	waitFor(t, term, "still-alive", 2)
}

// clear in one pane must not wipe its neighbor.
func TestClearStaysInsideItsPane(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("echo left-side\r"))
	waitFor(t, term, "left-side", 2)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte("clear; echo right-side\r"))
	waitFor(t, term, "right-side", 1)
	waitFor(t, term, "left-side", 2)
}

// A shell exiting in a background tab of another workspace is cleaned up,
// and closing a workspace's last tab doesn't end the whole session.
func TestExitAcrossWorkspaces(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'W'}) // workspace 2
	waitFor(t, term, "ws-2", 1)
	a.FeedInput([]byte("exit\r"))
	waitGone(t, term, "ws-2")
	select {
	case <-a.Done():
		t.Fatal("session ended although workspace 1 is still open")
	case <-time.After(100 * time.Millisecond):
	}
	a.FeedInput([]byte("echo back-in-main\r"))
	waitFor(t, term, "back-in-main", 2)

	a.FeedInput([]byte("exit\r"))
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not end after its last shell exited")
	}
}

// Keys that close the help overlay or cancel a prompt must not leak into
// the shell.
func TestOverlaysSwallowEscapeSequences(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, '?'})
	waitFor(t, term, "Split left / right", 1)
	a.FeedInput([]byte("\x1b[A")) // up arrow closes help
	waitGone(t, term, "Split left / right")

	a.FeedInput([]byte{prefix, ','})
	waitFor(t, term, "rename tab", 1)
	a.FeedInput([]byte("\x1b[B")) // down arrow cancels the prompt
	waitGone(t, term, "rename tab")

	a.FeedInput([]byte{prefix, ','})
	a.FeedInput([]byte("\x15logs\r")) // Ctrl-U, type, Enter
	waitFor(t, term, "1:logs", 1)

	a.FeedInput([]byte{prefix, 'S'})
	waitFor(t, term, "rename session", 1)
	a.FeedInput([]byte("\x15my-proj\r"))
	waitGone(t, term, "rename session")
	if got := a.manager.SessionName; got != "my-proj" {
		t.Errorf("SessionName = %q, want my-proj", got)
	}

	a.FeedInput([]byte("echo ok\r"))
	waitFor(t, term, "slat$ echo ok", 1) // no stray "[A"/"[B" before it
}

func TestScrollMode(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("for i in $(seq 1 200); do echo row-$i; done\r"))
	waitFor(t, term, "row-200", 1)

	a.FeedInput([]byte{prefix, '['})
	waitFor(t, term, "SCROLL 0/", 1)
	a.FeedInput([]byte("g")) // oldest line
	waitFor(t, term, "row-1\n", 1)
	waitGone(t, term, "row-200")

	// New output doesn't move a scrolled-back view.
	a.mu.Lock()
	// Already under the lock: the helpers below take it themselves, so
	// calling one here would deadlock against this very line.
	a.manager.ActivePane().Write([]byte("echo fresh-output\r"))
	a.mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	waitFor(t, term, "row-1\n", 1)
	waitGone(t, term, "fresh-output")

	// Search down from the top, then up again with N.
	a.FeedInput([]byte("?row-150\r"))
	waitFor(t, term, "row-150", 1)
	waitGone(t, term, "row-1\n")
	a.FeedInput([]byte("/ROW-99\r")) // capitals: case-sensitive, no match
	waitFor(t, term, "not found: ROW-99", 1)

	a.FeedInput([]byte("q"))
	waitGone(t, term, "SCROLL")
	waitFor(t, term, "fresh-output", 2)

	// Keys typed in scroll mode never reach the shell.
	a.FeedInput([]byte{prefix, '['})
	waitFor(t, term, "SCROLL", 1)
	a.FeedInput([]byte("kkjj\x1b[A\x1b"))
	waitGone(t, term, "SCROLL")
	a.FeedInput([]byte("echo clean\r"))
	waitFor(t, term, "slat$ echo clean", 1)
}

// The app's fields belong to its mutex, and a test runs beside the render
// loop, so tests read them through these rather than directly: without the
// lock the race detector is right to complain, and intermittently does.

func activePanes(a *App) []*pane.Pane {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.manager.ActivePanes()
}

func activePane(a *App) *pane.Pane {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.manager.ActivePane()
}

// scrollState reports whether scroll mode is open and where its view sits.
func scrollState(a *App) (open bool, top int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scroll == nil {
		return false, 0
	}
	return true, a.scroll.top
}

func TestStackedMode(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "pane 3/3", 1)

	// Toggle stacked mode on (<prefix> t).
	a.FeedInput([]byte{prefix, 't'})
	waitFor(t, term, "STACK", 1)
	waitFor(t, term, "▸ 3:", 1)

	// Jump across stack elements with j/k and o/O without leaving stacked mode.
	a.FeedInput([]byte{prefix, 'j'})
	waitFor(t, term, "pane 1/3", 1)
	waitFor(t, term, "▸ 1:", 1)

	a.FeedInput([]byte{prefix, 'k'})
	waitFor(t, term, "pane 3/3", 1)

	// Click stack header row 2 (y=2, 1-based) to jump directly to pane 2.
	a.FeedInput([]byte("\x1b[<0;5;2M"))
	waitFor(t, term, "pane 2/3", 1)

	// Toggle stacked mode off (<prefix> t).
	a.FeedInput([]byte{prefix, 't'})
	waitGone(t, term, "STACK")
}

func TestMultiBytePrefixF13(t *testing.T) {
	pk, err := input.ParsePrefix("F13")
	if err != nil {
		t.Fatal(err)
	}
	a, term := start(t, func(cfg *config.Config) {
		cfg.Prefix = "F13"
		cfg.PrefixKey = pk
	})

	// Unrelated escape sequence followed by F13 + 'v' in one chunk.
	a.FeedInput([]byte("\x1b[A\x1b[25~v"))
	waitFor(t, term, "pane 2/2", 1)
	waitFor(t, term, "slat$", 2)

	// F13 + F13 in pane 2/2 (clean shell) sends the F13 sequence to the shell.
	a.FeedInput([]byte("cat -v\r"))
	waitFor(t, term, "cat -v", 1)
	a.FeedInput([]byte("\x1b[25~\x1b[25~\r"))
	waitFor(t, term, "^[[25~", 1)
	a.FeedInput([]byte{0x04}) // Ctrl-D to exit cat

	// F13 + '?' shows "F13" in help title.
	a.FeedInput([]byte("\x1b[25~?"))
	waitFor(t, term, "slat · keys after F13", 1)
	a.FeedInput([]byte{'q'}) // close help
	waitGone(t, term, "slat · keys after F13")

	// F13 + arrow left selects pane 1/2.
	a.FeedInput([]byte("\x1b[25~\x1b[D"))
	waitFor(t, term, "pane 1/2", 1)

	// F13 in scroll mode exits scroll mode and activates prefix.
	a.FeedInput([]byte("\x1b[25~["))
	waitFor(t, term, "SCROLL", 1)
	a.FeedInput([]byte("\x1b[25~"))
	waitGone(t, term, "SCROLL")
	waitFor(t, term, "PREFIX", 1)
}

func TestToggleStatusBar(t *testing.T) {
	a, term := start(t)
	waitFor(t, term, "pane 1/1", 1)

	// Toggle status bar off (<prefix> b).
	a.FeedInput([]byte{prefix, 'b'})
	waitGone(t, term, "pane 1/1")
	if area := a.paneArea(); area.Rows != 30 {
		t.Fatalf("paneArea.Rows = %d, want 30 when status bar hidden", area.Rows)
	}

	// Toggle status bar back on (<prefix> b).
	a.FeedInput([]byte{prefix, 'b'})
	waitFor(t, term, "pane 1/1", 1)
	if area := a.paneArea(); area.Rows != 29 {
		t.Fatalf("paneArea.Rows = %d, want 29 when status bar shown", area.Rows)
	}
}

func TestLayoutPicker(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "pane 2/2", 1)

	// Open layout picker (<prefix> l), filter for master-right, press Enter.
	a.FeedInput([]byte{prefix, 'l'})
	waitFor(t, term, "layouts (8/8)", 1)
	a.FeedInput([]byte("master-right"))
	waitFor(t, term, "▸ master-right", 1)
	a.FeedInput([]byte{'\r'})
	waitGone(t, term, "layouts (")

	a.mu.Lock()
	mode := a.manager.ActiveTab().LayoutMode
	a.mu.Unlock()
	if mode != "master-right" {
		t.Fatalf("LayoutMode = %q, want %q", mode, "master-right")
	}
}

func TestSaveSessionWithOverwriteConfirm(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	a, term := start(t)
	waitFor(t, term, "pane 1/1", 1)

	// Save session as "demo" (<prefix> E -> Ctrl-U -> "demo" -> Enter).
	a.FeedInput([]byte{prefix, 'E'})
	waitFor(t, term, "save session as", 1)
	a.FeedInput([]byte{0x15})
	a.FeedInput([]byte("demo\r"))
	waitFor(t, term, "saved session demo", 1)

	target := filepath.Join(cfgHome, "slat", "sessions", "demo.toml")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected saved preset at %s: %v", target, err)
	}

	// Trigger save again with the same name -> should ask before overwriting.
	a.FeedInput([]byte{prefix, 'E'})
	waitFor(t, term, "save session as", 1)
	a.FeedInput([]byte{0x15})
	a.FeedInput([]byte("demo\r"))
	waitFor(t, term, "overwrite demo.toml? [y/N]", 1)

	// Press 'n' to cancel.
	a.FeedInput([]byte{'n'})
	waitFor(t, term, "save canceled", 1)

	// Trigger again and press 'y' to overwrite.
	a.FeedInput([]byte{prefix, 'E'})
	waitFor(t, term, "save session as", 1)
	a.FeedInput([]byte{0x15})
	a.FeedInput([]byte("demo\r"))
	waitFor(t, term, "overwrite demo.toml? [y/N]", 1)
	a.FeedInput([]byte{'y'})
	waitFor(t, term, "saved session demo", 1)
}

func TestAddPaneHonorsLayout(t *testing.T) {
	a, term := start(t)
	waitFor(t, term, "pane 1/1", 1)

	// Switch to master-left layout, then add two panes with <prefix> a.
	a.mu.Lock()
	a.manager.ApplyLayout("master-left")
	a.mu.Unlock()

	a.FeedInput([]byte{prefix, 'a'})
	waitFor(t, term, "pane 2/2", 1)
	a.FeedInput([]byte{prefix, 'a'})
	waitFor(t, term, "pane 3/3", 1)

	a.mu.Lock()
	tab := a.manager.ActiveTab()
	mode := tab.LayoutMode
	split := tab.Layout.Split
	rightSplit := tab.Layout.Children[1].Split
	a.mu.Unlock()

	if mode != "master-left" {
		t.Fatalf("LayoutMode = %q, want master-left", mode)
	}
	// master-left has vertical root split (left master | right stack) and horizontal stack split.
	if split != pane.SplitVertical || rightSplit != pane.SplitHorizontal {
		t.Fatalf("unexpected tree splits for master-left: root=%v right=%v", split, rightSplit)
	}
}
