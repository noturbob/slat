package daemon_test

import (
	"strings"
	"testing"

	"github.com/noturbob/slat/internal/control"
)

// The Windows path end to end: a ConPTY running the console shell, the
// AF_UNIX socket, and the control commands over it. Status there comes
// from output timing alone (ConPTY has no foreground process group), so
// this checks the parts that do work rather than pretending otherwise.
func TestWindowsSession(t *testing.T) {
	sock := session(t)

	ls := do(t, sock, control.Request{Cmd: "ls"})
	if len(ls.Panes) != 1 {
		t.Fatalf("ls returned %+v", ls.Panes)
	}
	first := ls.Panes[0]
	if first.Cols == 0 || first.Rows == 0 {
		t.Errorf("pane has no size: %+v", first)
	}
	pane := "1"

	// The shell runs a command and we can read what it printed.
	do(t, sock, control.Request{Cmd: "send", Pane: pane, Data: "echo hello-from-slat\r"})
	w := do(t, sock, control.Request{Cmd: "wait", Pane: pane, For: "text=hello-from-slat", Timeout: "60s"})
	if w.Matched == nil || !*w.Matched {
		t.Fatalf("the shell's output never arrived: %+v", w)
	}
	cap := do(t, sock, control.Request{Cmd: "capture", Pane: pane, History: true})
	if !strings.Contains(strings.Join(cap.Lines, "\n"), "hello-from-slat") {
		t.Errorf("capture missed it: %q", cap.Lines)
	}

	// A quiet pane reads as idle, which is what `wait --for idle` needs.
	w = do(t, sock, control.Request{Cmd: "wait", Pane: pane, For: "idle", Timeout: "30s"})
	if w.Matched == nil || !*w.Matched {
		t.Errorf("wait --for idle: %+v", w)
	}

	// Splitting and closing work, and the second shell really starts.
	np := do(t, sock, control.Request{Cmd: "pane-new", Split: "h"})
	if np.Pane == nil {
		t.Fatalf("pane new failed: %v", np.Error)
	}
	if n := len(do(t, sock, control.Request{Cmd: "ls"}).Panes); n != 2 {
		t.Errorf("ls shows %d panes, want 2", n)
	}
	if resp := do(t, sock, control.Request{Cmd: "pane-close", Pane: "2"}); resp.Code != 0 {
		t.Errorf("pane close: %+v", resp)
	}
}
