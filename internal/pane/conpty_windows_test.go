package pane

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The ConPTY path, checked the way slat uses it (through Pane) and then
// raw, in one test and in that order: two runs in one process could
// interfere, and "ConPTY doesn't work here" needs telling apart from
// "Pane breaks it". Everything is logged because a CI runner gives no
// other way to look.
func TestConPTY(t *testing.T) {
	t.Logf("shell=%q", defaultShell())

	p, err := New(1, 24, 80, "", "", 200, nil)
	if err != nil {
		t.Fatalf("pane.New: %v", err)
	}
	defer p.Close()
	w := p.proc.(*winPTY)
	t.Logf("pane: console=%#x pid=%d in=%#x out=%#x", w.console, w.pid, w.in, w.out)

	if paneText(p, "", 10*time.Second) {
		// It works: type a command and read the answer back.
		if _, err := p.Write([]byte("echo hello-conpty\r\n")); err != nil {
			t.Fatalf("writing to the pane: %v", err)
		}
		if !paneText(p, "hello-conpty", 15*time.Second) {
			t.Errorf("the command's output never arrived; the pane held:\n%s",
				strings.Join(p.Capture(0, true), "\n"))
		}
		// The first bug this code had: the child attached to slat's own
		// console instead of the pseudoconsole, which looked like a working
		// shell whose output went to the wrong terminal.
		if pids := consoleProcesses(t); slices.Contains(pids, uint32(w.pid)) {
			t.Errorf("the shell (pid %d) is on this process's console, not its pseudoconsole: %v", w.pid, pids)
		}
		// Resizing a live console must not upset it; panes are resized on
		// every split.
		if err := p.proc.Resize(30, 100); err != nil {
			t.Errorf("resize: %v", err)
		}
		return
	}

	t.Errorf("no output through Pane in 10s: dead=%v child exit=%d (259 = running)",
		p.Dead(), exitCode(w))
	t.Logf("console processes: %v (this process=%d)", consoleProcesses(t), windows.GetCurrentProcessId())

	// Three probes, read raw, to place the blame:
	//   ping  — writes for seconds and never reads input
	//   /k    — runs a command and stays
	//   plain — the interactive shell slat actually wants
	for _, probe := range []struct{ name, cmd string }{
		{"ping", defaultShell() + " /c ping -n 3 127.0.0.1"},
		{"keep", defaultShell() + " /k echo STAYING"},
		{"interactive", ""},
	} {
		t.Log(drain(t, probe.name, probe.cmd, 10*time.Second))
	}
}

// drain starts a program on a ConPTY and reports everything it wrote
// within the timeout, how long it lived and how it ended.
func drain(t *testing.T, name, shell string, within time.Duration) string {
	start := time.Now()
	proc, err := startPTY(shell, "", 24, 80)
	if err != nil {
		return name + ": startPTY: " + err.Error()
	}
	defer proc.Terminate()

	out := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := proc.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				b.WriteString(" <read ended: " + err.Error() + ">")
				break
			}
			if b.Len() > 2048 {
				break
			}
		}
		out <- b.String()
	}()
	select {
	case s := <-out:
		return fmt.Sprintf("%s: after %s exit=%d %q",
			name, time.Since(start).Round(time.Millisecond), exitCode(proc.(*winPTY)), s)
	case <-time.After(within):
		return fmt.Sprintf("%s: still reading after %s exit=%d (259 = running)",
			name, within, exitCode(proc.(*winPTY)))
	}
}

// paneText reports whether the pane's text contains s (any text, when s is
// empty) before the deadline.
func paneText(p *Pane, s string, within time.Duration) bool {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); {
		text := strings.Join(p.Capture(0, true), "\n")
		if s == "" && strings.TrimSpace(text) != "" || s != "" && strings.Contains(text, s) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func exitCode(w *winPTY) uint32 {
	var code uint32
	windows.GetExitCodeProcess(w.proc, &code)
	return code
}

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("GetConsoleProcessList")

// consoleProcesses lists what shares this test's console. A pane's shell
// appearing here was never attached to its pseudoconsole.
func consoleProcesses(t *testing.T) []uint32 {
	var pids [64]uint32
	r, _, e := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if r == 0 {
		t.Logf("GetConsoleProcessList: %v", e)
		return nil
	}
	return pids[:min(int(r), len(pids))]
}
