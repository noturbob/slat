//go:build !windows

package pane

import (
	"strings"
	"testing"
	"time"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The pane can tell "the shell is waiting" from "a program is running",
// which is what pane status and `slat wait` are built on.
func TestForegroundAndCapture(t *testing.T) {
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")
	p, err := New(1, 10, 40, "/bin/sh", "", 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	waitFor(t, "the shell to be idle", func() bool {
		_, name, isShell := p.Foreground()
		return isShell && name == "sh"
	})

	p.Write([]byte("echo marker-line\n"))
	waitFor(t, "output", func() bool {
		return strings.Contains(strings.Join(p.Capture(0, false), "\n"), "marker-line")
	})
	if time.Since(p.Activity()) > time.Second {
		t.Error("Activity did not move with the output")
	}

	p.Write([]byte("sleep 3\n"))
	waitFor(t, "sleep to take the terminal", func() bool {
		_, name, isShell := p.Foreground()
		return name == "sleep" && !isShell
	})

	// Capture: last line only, and scrollback beyond the screen.
	for i := 0; i < 20; i++ {
		p.Write([]byte("\003")) // Ctrl-C the sleep
		break
	}
	waitFor(t, "the shell to come back", func() bool {
		_, _, isShell := p.Foreground()
		return isShell
	})
	p.Write([]byte("for i in 1 2 3 4 5 6 7 8 9; do echo line-$i; done\n"))
	waitFor(t, "9 lines", func() bool {
		return strings.Contains(strings.Join(p.Capture(0, false), "\n"), "line-9")
	})
	if got := p.Capture(2, false); len(got) != 2 {
		t.Errorf("Capture(2) returned %d lines: %q", len(got), got)
	}
	screen := strings.Join(p.Capture(0, false), "\n")
	full := strings.Join(p.Capture(0, true), "\n")
	if strings.Contains(screen, "marker-line") {
		t.Error("marker-line should have scrolled off the 10-row screen")
	}
	if !strings.Contains(full, "marker-line") {
		t.Error("marker-line should still be in the scrollback capture")
	}
}
