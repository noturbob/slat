package pane

import (
	"fmt"
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

	// A one-shot command, read raw: this separates "the output path is
	// broken" from "the interactive shell doesn't stay alive".
	t.Logf("one-shot: %q", drain(t, defaultShell()+" /c echo SLAT-MARKER", 6*time.Second))
	// And the interactive shell, read raw, for as long as it lives.
	t.Logf("interactive: %q", drain(t, "", 6*time.Second))
}

// drain starts a program on a ConPTY and returns everything it writes
// within the timeout, along with how it ended.
func drain(t *testing.T, shell string, within time.Duration) string {
	proc, err := startPTY(shell, "", 24, 80)
	if err != nil {
		return "startPTY: " + err.Error()
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
		return s
	case <-time.After(within):
		return "<still reading> exit=" + fmt.Sprint(exitCode(proc.(*winPTY)))
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
