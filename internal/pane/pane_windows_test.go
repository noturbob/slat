package pane

import (
	"strings"
	"testing"
	"time"
)

// The ConPTY path at its smallest: start the console shell, type a command,
// read its output back out of the pane's emulator. When this fails it says
// what the pane did hold, because there is no other way to see it from CI.
func TestConPTYRunsTheConsoleShell(t *testing.T) {
	p, err := New(1, 24, 80, "", "", 200, nil)
	if err != nil {
		t.Fatalf("starting a pane: %v", err)
	}
	defer p.Close()

	// Wait for the shell to say anything at all before typing at it.
	if !waitFor(t, p, "", 10*time.Second) {
		t.Fatalf("the shell produced no output in 10s (dead=%v)", p.Dead())
	}
	if _, err := p.Write([]byte("echo hello-conpty\r\n")); err != nil {
		t.Fatalf("writing to the pane: %v", err)
	}
	if !waitFor(t, p, "hello-conpty", 15*time.Second) {
		t.Errorf("the command's output never arrived (dead=%v); the pane held:\n%s",
			p.Dead(), strings.Join(p.Capture(0, true), "\n"))
	}

	// Resizing a live console must not upset it.
	if err := p.proc.Resize(30, 100); err != nil {
		t.Errorf("resize: %v", err)
	}
	if _, _, rows, cols := p.Rect(); rows == 0 || cols == 0 {
		t.Errorf("pane has no size: %dx%d", cols, rows)
	}
}

// waitFor reports whether the pane's text contains s (any output, when s is
// empty) before the deadline.
func waitFor(t *testing.T, p *Pane, s string, within time.Duration) bool {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); {
		text := strings.Join(p.Capture(0, true), "\n")
		if s == "" && strings.TrimSpace(text) != "" || s != "" && strings.Contains(text, s) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
