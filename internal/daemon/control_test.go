//go:build !windows

package daemon_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
)

// posixShell keeps the prompt predictable across machines.
func posixShell(c *config.Config) { c.Shell = "/bin/sh" }

func TestControlLifecycle(t *testing.T) {
	sock := session(t, posixShell)

	// ls: the session starts with one pane, and it is the active one.
	ls := do(t, sock, control.Request{Cmd: "ls"})
	if len(ls.Panes) != 1 || !ls.Panes[0].Active {
		t.Fatalf("ls returned %+v", ls.Panes)
	}
	first := fmt.Sprint(ls.Panes[0].Pane)
	statusEventually(t, sock, first, "idle")

	// run: send a command and wait for it to finish.
	do(t, sock, control.Request{Cmd: "send", Pane: first, Data: "echo hello-from-agent\r"})
	w := do(t, sock, control.Request{Cmd: "wait", Pane: first, For: "idle", Timeout: "15s"})
	if w.Matched == nil || !*w.Matched {
		t.Fatalf("wait --for idle did not match: %+v", w)
	}
	cap := do(t, sock, control.Request{Cmd: "capture", Pane: first})
	if !strings.Contains(strings.Join(cap.Lines, "\n"), "hello-from-agent") {
		t.Errorf("capture missed the output: %q", cap.Lines)
	}

	// pane new: a second pane, running something, without stealing focus.
	np := do(t, sock, control.Request{Cmd: "pane-new", Split: "h", Command: "sleep 30"})
	if np.Pane == nil {
		t.Fatalf("pane new failed: %v", np.Error)
	}
	second := fmt.Sprint(np.Pane.Pane)
	// The reported program settles on sleep, not the sh that forked it.
	fg := ""
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		fg = statusEventually(t, sock, second, "working").Pane.Foreground
		if fg == "sleep" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if fg != "sleep" {
		t.Errorf("foreground = %q, want sleep", fg)
	}
	if ls := do(t, sock, control.Request{Cmd: "ls"}); len(ls.Panes) != 2 || !ls.Panes[0].Active {
		t.Errorf("pane new stole focus or miscounted: %+v", ls.Panes)
	}

	// wait --for text: matches output that arrives later.
	go func() {
		time.Sleep(300 * time.Millisecond)
		control.Do(sock, control.Request{Cmd: "send", Pane: first, Data: "echo the-marker\r"}, 5*time.Second)
	}()
	w = do(t, sock, control.Request{Cmd: "wait", Pane: first, For: "text=the-mark\\w+", Timeout: "15s"})
	if w.Matched == nil || !*w.Matched {
		t.Errorf("wait --for text did not match: %+v", w)
	}

	// timeout: exit code 2, with the pane's state still reported.
	w = do(t, sock, control.Request{Cmd: "wait", Pane: second, For: "idle", Timeout: "300ms"})
	if w.Code != control.CodeTimeout || w.Matched == nil || *w.Matched {
		t.Errorf("wait on a busy pane should time out: %+v", w)
	}

	// Ctrl-C reaches the program: sleep dies and the shell comes back.
	do(t, sock, control.Request{Cmd: "send", Pane: second, Data: "\x03"})
	statusEventually(t, sock, second, "idle")

	// close: the pane goes away and further commands say so.
	do(t, sock, control.Request{Cmd: "pane-close", Pane: second})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(do(t, sock, control.Request{Cmd: "ls"}).Panes) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if resp := do(t, sock, control.Request{Cmd: "send", Pane: second, Data: "x"}); resp.Code != control.CodePaneGone {
		t.Errorf("send to a closed pane: code %d, want %d (%+v)", resp.Code, control.CodePaneGone, resp)
	}
	// Every command that takes a pane reports a closed one the same way,
	// so a script can act on the code instead of matching the message.
	for _, req := range []control.Request{
		{Cmd: "send", Pane: second, Data: "x"},
		{Cmd: "status", Pane: second},
		{Cmd: "capture", Pane: second},
		{Cmd: "wait", Pane: second, For: "idle", Timeout: "5s"},
		{Cmd: "pane-close", Pane: second},
	} {
		if resp := do(t, sock, req); resp.Code != control.CodePaneGone {
			t.Errorf("%s on a closed pane: code %d, want %d (%+v)",
				req.Cmd, resp.Code, control.CodePaneGone, resp)
		}
	}
	// An id that never existed is a different mistake.
	if resp := do(t, sock, control.Request{Cmd: "status", Pane: "99"}); resp.Code != control.CodeError {
		t.Errorf("status on a bogus id: code %d, want %d", resp.Code, control.CodeError)
	}

	// capture --lines skips the blank rows below the last output.
	cap = do(t, sock, control.Request{Cmd: "capture", Pane: first, Lines: 2})
	if len(cap.Lines) == 0 || strings.TrimSpace(cap.Lines[len(cap.Lines)-1]) == "" {
		t.Errorf("capture --lines returned blank tail: %q", cap.Lines)
	}
}

// A pane stopped on a question reads as "input", not "working", which is
// the distinction agents need.
func TestStatusInput(t *testing.T) {
	sock := session(t, posixShell)
	first := fmt.Sprint(do(t, sock, control.Request{Cmd: "ls"}).Panes[0].Pane)
	statusEventually(t, sock, first, "idle")

	// A builtin asking a question: the pane's own shell is in the
	// foreground, so only the prompt text gives this away.
	do(t, sock, control.Request{Cmd: "send", Pane: first,
		Data: `printf 'Overwrite everything? [y/N] '; read builtin` + "\r"})
	w0 := do(t, sock, control.Request{Cmd: "wait", Pane: first, For: "input", Timeout: "20s"})
	if w0.Matched == nil || !*w0.Matched {
		t.Fatalf("a shell builtin waiting for input was missed: %+v", w0)
	}
	do(t, sock, control.Request{Cmd: "send", Pane: first, Data: "n\r"})
	statusEventually(t, sock, first, "idle")

	// A child process stops on a question: the pane's own shell is not in
	// the foreground, so this is "input", not "idle".
	do(t, sock, control.Request{Cmd: "send", Pane: first,
		Data: `sh -c 'printf "Overwrite everything? [y/N] "; read a'` + "\r"})
	statusEventually(t, sock, first, "working")

	w := do(t, sock, control.Request{Cmd: "wait", Pane: first, For: "input", Timeout: "30s"})
	if w.Matched == nil || !*w.Matched {
		t.Fatalf("wait --for input did not match: %+v", w)
	}
	if w.Pane == nil || !strings.Contains(w.Pane.Last, "Overwrite everything?") {
		t.Errorf("the reported last line was %q", w.Pane.Last)
	}

	// Answering it lets the shell continue.
	do(t, sock, control.Request{Cmd: "send", Pane: first, Data: "n\r"})
	statusEventually(t, sock, first, "idle")
}

// The daemon runs the [agent] hook when a pane starts waiting for input,
// once, on the change — that is what tells a human an agent is stuck.
func TestInputHookFires(t *testing.T) {
	log := filepath.Join(t.TempDir(), "hook.log")
	sock := session(t, posixShell, func(c *config.Config) {
		c.Agent.OnInput = "echo pane %p %s >> " + log
	})
	first := fmt.Sprint(do(t, sock, control.Request{Cmd: "ls"}).Panes[0].Pane)
	statusEventually(t, sock, first, "idle")
	do(t, sock, control.Request{Cmd: "send", Pane: first,
		Data: `sh -c 'printf "Continue? [y/N] "; read a'` + "\r"})

	var got string
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(log); err == nil && len(b) > 0 {
			got = string(b)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if want := "pane " + first + " input\n"; got != want {
		t.Errorf("hook log = %q, want %q (once only)", got, want)
	}
}

// The workflow an agent actually uses: start a command in a new pane, wait
// for it, read the output. `wait` must not return before the command has
// even started — a fresh pane that hasn't printed its prompt yet is
// starting up, not idle.
func TestNewPaneThenWaitForIdle(t *testing.T) {
	sock := session(t, posixShell)
	np := do(t, sock, control.Request{Cmd: "pane-new", Command: "sleep 2; echo done-sleeping"})
	if np.Pane == nil {
		t.Fatalf("pane new failed: %v", np.Error)
	}
	pane := fmt.Sprint(np.Pane.Pane)

	start := time.Now()
	w := do(t, sock, control.Request{Cmd: "wait", Pane: pane, For: "idle", Timeout: "30s"})
	if w.Matched == nil || !*w.Matched {
		t.Fatalf("wait --for idle: %+v", w)
	}
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Errorf("wait returned after %s, before the command could finish", elapsed)
	}
	cap := do(t, sock, control.Request{Cmd: "capture", Pane: pane, History: true})
	if !strings.Contains(strings.Join(cap.Lines, "\n"), "done-sleeping") {
		t.Errorf("the command's output is missing: %q", cap.Lines)
	}
}
