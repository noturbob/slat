package pane

import (
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every step of the ConPTY setup, reported. A CI runner gives no other way
// to see why a pane came up empty, and the failure modes look identical
// from the outside: attached-but-silent and never-attached both read as
// "no output".
func TestConPTYDiagnostics(t *testing.T) {
	proc, err := startPTY("", "", 24, 80)
	if err != nil {
		t.Fatalf("startPTY: %v", err)
	}
	p := proc.(*winPTY)
	defer p.Terminate()
	t.Logf("shell=%q console=%#x pid=%d in=%#x out=%#x",
		defaultShell(), p.console, p.pid, p.in, p.out)

	// Does the child share this test's console? If it does, it was never
	// attached to the pseudoconsole — that is the whole question.
	var pids [64]uint32
	if n, err := getConsoleProcessList(&pids[0], uint32(len(pids))); err == nil {
		t.Logf("processes on this test's console: %v (ours=%d, child=%d)",
			pids[:min(int(n), len(pids))], windows.GetCurrentProcessId(), p.pid)
	} else {
		t.Logf("GetConsoleProcessList: %v", err)
	}

	type result struct {
		n   int
		err error
	}
	buf := make([]byte, 4096)
	ch := make(chan result, 1)
	go func() {
		n, err := p.Read(buf)
		ch <- result{n, err}
	}()
	select {
	case r := <-ch:
		t.Logf("first read: n=%d err=%v data=%q", r.n, r.err, buf[:r.n])
		if r.n > 0 {
			return // working: the shell's output reached us
		}
	case <-time.After(8 * time.Second):
		t.Log("first read: nothing arrived in 8s")
	}

	var code uint32
	windows.GetExitCodeProcess(p.proc, &code)
	t.Errorf("no output from the shell; child exit code=%d (259 means still running)", code)
}

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("GetConsoleProcessList")

func getConsoleProcessList(list *uint32, count uint32) (uint32, error) {
	r, _, e := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(list)), uintptr(count))
	if r == 0 {
		return 0, e
	}
	return uint32(r), nil
}
