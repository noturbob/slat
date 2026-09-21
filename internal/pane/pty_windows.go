package pane

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winPTY is a program on a ConPTY. The pseudoconsole reads the program's
// input from one pipe and writes its output to another; we keep the other
// end of each. The program is put in a job object so that closing a pane
// takes everything it started with it, the way SIGHUP to a process group
// does on Unix.
type winPTY struct {
	console windows.Handle // the pseudoconsole
	job     windows.Handle // the program and its children
	proc    windows.Handle
	pid     int

	// Raw handles, not os.File: these are synchronous pipes, and Go's
	// file machinery (poller registration, finalizers) has no business on
	// handles whose lifetime the pseudoconsole shares.
	in  windows.Handle // we write the program's input here
	out windows.Handle // we read the program's output here
	dir string

	closeOnce sync.Once
	exited    chan struct{}
}

func startPTY(shell, dir string, rows, cols int) (ptyProcess, error) {
	if shell == "" {
		shell = defaultShell()
	}
	// Two pipes: the console's input (we write the far end) and its output
	// (we read the far end).
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return nil, err
	}
	size := windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))}
	var console windows.Handle
	if err := windows.CreatePseudoConsole(size, inRead, outWrite, 0, &console); err != nil {
		for _, h := range []windows.Handle{inRead, inWrite, outRead, outWrite} {
			windows.CloseHandle(h)
		}
		return nil, fmt.Errorf("ConPTY: %w (Windows 10 1809 or later is required)", err)
	}

	// Both of the console's own ends belong to the console host now.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)

	p := &winPTY{
		console: console,
		in:      inWrite,
		out:     outRead,
		dir:     dir,
		exited:  make(chan struct{}),
	}
	if err := p.spawn(shell, dir); err != nil {
		p.release()
		return nil, err
	}
	go p.reap()
	return p, nil
}

var procUpdateProcThreadAttribute = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("UpdateProcThreadAttribute")

// spawn starts the shell attached to the pseudoconsole.
func (p *winPTY) spawn(shell, dir string) error {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()

	// This attribute's value *is* the pseudoconsole handle, not a pointer
	// to it, which is why this goes through the raw call: x/sys's wrapper
	// takes an unsafe.Pointer and retains it as a Go pointer, and a handle
	// must never be mistaken for one.
	if r, _, e := procUpdateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(attrs.List())), 0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		uintptr(p.console), unsafe.Sizeof(p.console), 0, 0,
	); r == 0 {
		return fmt.Errorf("attaching the pseudoconsole: %w", e)
	}

	// STARTF_USESTDHANDLES with all three handles left empty. Without it,
	// CreateProcess copies *our* standard handles into the child, and this
	// process is a daemon whose handles are pipes or the null device: the
	// shell would attach to the console (its title even changes) but write
	// into nothing and read EOF at once. Empty handles plus the console
	// attribute make the console host give the child the pseudoconsole's
	// own, which is the whole point.
	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:    uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attrs.List(),
	}
	cmdline, err := windows.UTF16PtrFromString(quoteExe(shell))
	if err != nil {
		return err
	}
	var cwd *uint16
	if dir != "" {
		if cwd, err = windows.UTF16PtrFromString(dir); err != nil {
			return err
		}
	}
	env, err := envBlock()
	if err != nil {
		return err
	}

	var pi windows.ProcessInformation
	err = windows.CreateProcess(nil, cmdline, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		env, cwd, &si.StartupInfo, &pi)
	if err != nil {
		return fmt.Errorf("failed to start %s: %w", shell, err)
	}
	windows.CloseHandle(pi.Thread)
	p.proc, p.pid = pi.Process, int(pi.ProcessId)

	// Best effort: a job whose closing kills the tree. Without it a pane
	// still closes, it just may leave a child of the shell behind.
	if job, err := windows.CreateJobObject(nil, nil); err == nil {
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err == nil &&
			windows.AssignProcessToJobObject(job, p.proc) == nil {
			p.job = job
		} else {
			windows.CloseHandle(job)
		}
	}
	return nil
}

// reap waits for the program, then releases the pseudoconsole and the
// output pipe, so the pane's read loop sees EOF and the pane is marked
// dead. The console holds the write end, so letting it go is what
// actually unblocks a read in progress.
func (p *winPTY) reap() {
	windows.WaitForSingleObject(p.proc, windows.INFINITE)
	close(p.exited)
	p.Close()
}

// Read blocks in ReadFile until the program writes something. A closed
// pseudoconsole shows up as a broken pipe, which is this pane's EOF.
func (p *winPTY) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	var n uint32
	if err := windows.ReadFile(p.out, b, &n, nil); err != nil {
		if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_INVALID_HANDLE {
			return int(n), io.EOF
		}
		return int(n), err
	}
	return int(n), nil
}

func (p *winPTY) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	var n uint32
	err := windows.WriteFile(p.in, b, &n, nil)
	return int(n), err
}

func (p *winPTY) Pid() int { return p.pid }
func (p *winPTY) Wait()    { <-p.exited }

func (p *winPTY) Resize(rows, cols int) error {
	return windows.ResizePseudoConsole(p.console,
		windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))})
}

// Close releases the pseudoconsole and the pipes. Closing the console
// tells the program its terminal is gone, which is how a shell is asked
// to exit.
func (p *winPTY) Close() error {
	p.closeOnce.Do(p.release)
	return nil
}

// release tears the console down in the one order that doesn't deadlock:
// the input first, then the console itself — which is what makes the
// console host exit and the output pipe report EOF, unblocking a read in
// progress — and only then our own end of the output.
func (p *winPTY) release() {
	windows.CloseHandle(p.in)
	if p.console != 0 {
		windows.ClosePseudoConsole(p.console)
	}
	windows.CloseHandle(p.out)
	if p.proc != 0 {
		windows.CloseHandle(p.proc)
	}
}

// Terminate closes the job, which kills the shell and everything it
// started; the pseudoconsole goes with Close.
func (p *winPTY) Terminate() {
	if p.job != 0 {
		windows.CloseHandle(p.job) // kills the tree: KILL_ON_JOB_CLOSE
		p.job = 0
	} else if p.proc != 0 {
		windows.TerminateProcess(p.proc, 1)
	}
	p.Close()
}

// Foreground has no answer on Windows: ConPTY has no foreground process
// group, so a pane's status there comes from output timing alone.
func (p *winPTY) Foreground() (int, string) { return 0, "" }

// Cwd reports the directory the pane started in. Following a shell's own
// cd would mean reading another process's memory on Windows, so this can
// be stale; it is better than nothing for `slat ls`.
func (p *winPTY) Cwd() string { return p.dir }

// quoteExe quotes a shell path that needs it. CreateProcess parses the
// command line itself, so C:\Program Files\PowerShell\7\pwsh.exe would
// otherwise be read as the program "C:\Program" with arguments.
func quoteExe(shell string) string {
	if strings.ContainsAny(shell, " \t") && !strings.HasPrefix(shell, `"`) {
		return `"` + shell + `"`
	}
	return shell
}

func defaultShell() string {
	if s := os.Getenv("COMSPEC"); s != "" {
		return s
	}
	return "cmd.exe"
}

// envBlock is os.Environ plus TERM, in the double-NUL-terminated block
// CreateProcess wants.
func envBlock() (*uint16, error) {
	env := append(os.Environ(), "TERM=xterm-256color")
	var b []uint16
	for _, e := range env {
		if strings.IndexByte(e, 0) >= 0 {
			continue
		}
		u, err := windows.UTF16FromString(e)
		if err != nil {
			return nil, err
		}
		b = append(b, u...)
	}
	b = append(b, 0)
	return &b[0], nil
}
