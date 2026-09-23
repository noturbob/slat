//go:build !windows

package daemon_test

import (
	"os"
	"strings"
	"testing"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
	sess "github.com/noturbob/slat/internal/session"
)

// The reboot case, through a real daemon: a session with output in it is
// stopped the way a shutdown stops it, and a second daemon brings it back.
func TestDaemonRestoresTheSession(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	sock := session(t, posixShell)
	ls := do(t, sock, control.Request{Cmd: "ls"})
	first := ls.Panes[0].Pane

	do(t, sock, control.Request{Cmd: "pane-new", Split: "v"})
	do(t, sock, control.Request{Cmd: "send", Pane: itoa(first), Data: "echo across-the-reboot\r"})
	do(t, sock, control.Request{Cmd: "wait", Pane: itoa(first), For: "idle", Timeout: "15s"})

	stopSession(t, sock) // as SIGTERM does

	path, err := sess.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the daemon saved nothing on the way out: %v", err)
	}

	// A second daemon, as the next boot would start.
	sock2 := session(t, posixShell)
	ls2 := do(t, sock2, control.Request{Cmd: "ls"})
	if len(ls2.Panes) != 2 {
		t.Fatalf("restored %d panes, want 2", len(ls2.Panes))
	}
	cap := do(t, sock2, control.Request{Cmd: "capture", Pane: itoa(ls2.Panes[0].Pane), Lines: 200, History: true})
	if !strings.Contains(strings.Join(cap.Lines, "\n"), "across-the-reboot") {
		t.Errorf("the output did not come back: %q", cap.Lines)
	}
}

// restore = false leaves nothing behind, so nothing can come back.
func TestDaemonWithoutRestore(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sock := session(t, posixShell, func(c *config.Config) { c.Restore = false })
	do(t, sock, control.Request{Cmd: "pane-new", Split: "v"})
	stopSession(t, sock)

	path, _ := sess.StatePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("restore = false still wrote a session (err = %v)", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
