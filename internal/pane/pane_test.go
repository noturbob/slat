//go:build !windows

package pane

import (
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/vt"
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

	if _, writeErr := p.Write([]byte("echo marker-line\n")); writeErr != nil {
		t.Fatal(writeErr)
	}
	waitFor(t, "output", func() bool {
		return strings.Contains(strings.Join(p.Capture(0, false), "\n"), "marker-line")
	})
	if time.Since(p.Activity()) > time.Second {
		t.Error("Activity did not move with the output")
	}

	if _, writeErr := p.Write([]byte("sleep 3\n")); writeErr != nil {
		t.Fatal(writeErr)
	}
	waitFor(t, "sleep to take the terminal", func() bool {
		_, name, isShell := p.Foreground()
		return name == "sleep" && !isShell
	})

	// Capture: last line only, and scrollback beyond the screen.
	if _, writeErr := p.Write([]byte("\003")); writeErr != nil { // Ctrl-C the sleep
		t.Fatal(writeErr)
	}
	waitFor(t, "the shell to come back", func() bool {
		_, _, isShell := p.Foreground()
		return isShell
	})
	if _, writeErr := p.Write([]byte("for i in 1 2 3 4 5 6 7 8 9; do echo line-$i; done\n")); writeErr != nil {
		t.Fatal(writeErr)
	}
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

func BenchmarkPaneLastLine(b *testing.B) {
	term := vt.New(120, 40)
	for i := 0; i < 40; i++ {
		if _, err := term.Write([]byte("line of terminal output with some text\r\n")); err != nil {
			b.Fatal(err)
		}
	}
	p := &Pane{term: term}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.LastLine()
	}
}

func BenchmarkPaneCaptureScreen(b *testing.B) {
	term := vt.New(120, 40)
	for i := 0; i < 40; i++ {
		if _, err := term.Write([]byte("line of terminal output with some text\r\n")); err != nil {
			b.Fatal(err)
		}
	}
	p := &Pane{term: term}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Capture(0, false)
	}
}
