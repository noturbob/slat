package pane

import (
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
		return
	}

	t.Errorf("no output through Pane in 10s: dead=%v child exit=%d (259 = running)",
		p.Dead(), exitCode(w))
	t.Logf("console processes: %v (this process=%d)", consoleProcesses(t), windows.GetCurrentProcessId())

	// Same thing without Pane, to place the blame.
	proc, err := startPTY("", "", 24, 80)
	if err != nil {
		t.Fatalf("raw startPTY: %v", err)
	}
	defer proc.Terminate()
	buf := make([]byte, 4096)
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := proc.Read(buf)
		ch <- result{n, err}
	}()
	select {
	case r := <-ch:
		t.Logf("raw read: n=%d err=%v data=%q", r.n, r.err, buf[:r.n])
	case <-time.After(8 * time.Second):
		t.Logf("raw read: nothing in 8s, child exit=%d", exitCode(proc.(*winPTY)))
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
