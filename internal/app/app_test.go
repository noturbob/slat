package app

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/vt"
)

// terminal stands in for the user's real terminal: it interprets
// everything slat sends, so tests can assert on what the user would see.
type terminal struct {
	mu  sync.Mutex
	emu *vt.Terminal
}

func (t *terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.emu.Write(p)
}

func (t *terminal) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.emu.String()
}

const prefix = 0x13 // Ctrl-S

func start(t *testing.T) (*App, *terminal) {
	t.Helper()
	t.Setenv("PS1", "slat$ ")
	t.Setenv("ENV", "") // keep sh from sourcing rc files that reset PS1
	cfg := config.DefaultConfig()
	cfg.Shell = "/bin/sh"
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
	waitFor(t, term, "rename tab:", 1)
	a.FeedInput([]byte("\x1b[B")) // down arrow cancels the prompt
	waitGone(t, term, "rename tab:")

	a.FeedInput([]byte{prefix, ','})
	a.FeedInput([]byte("\x15logs\r")) // Ctrl-U, type, Enter
	waitFor(t, term, "1:logs", 1)

	a.FeedInput([]byte("echo ok\r"))
	waitFor(t, term, "slat$ echo ok", 1) // no stray "[A"/"[B" before it
}
