package pane

import (
	"fmt"
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

	in  *os.File // we write the program's input here
	out *os.File // we read the program's output here
	dir string

	closeOnce sync.Once
	exited    chan struct{}
}

func startPTY(shell, dir string, rows, cols int) (ptyProcess, error) {
	if shell == "" {
		shell = defaultShell()
	}
	// Two pipes: the console's stdin (we write) and stdout (we read).
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		inRead.Close()
		inWrite.Close()
		return nil, err
	}

	size := windows.Coord{X: int16(max(cols, 1)), Y: int16(max(rows, 1))}
	var console windows.Handle
	err = windows.CreatePseudoConsole(size, windows.Handle(inRead.Fd()), windows.Handle(outWrite.Fd()), 0, &console)
	// The console holds its own references to these two ends now.
	inRead.Close()
	outWrite.Close()
	if err != nil {
		inWrite.Close()
		outRead.Close()
		return nil, fmt.Errorf("ConPTY: %w (Windows 10 1809 or later is required)", err)
	}

	p := &winPTY{console: console, in: inWrite, out: outRead, dir: dir, exited: make(chan struct{})}
	if err := p.spawn(shell, dir); err != nil {
		p.release()
		return nil, err
	}
	go p.reap()
	return p, nil
}

// spawn starts the shell attached to the pseudoconsole.
func (p *winPTY) spawn(shell, dir string) error {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// This attribute's value *is* the handle, not a pointer to it, so the
	// handle's bits are reinterpreted rather than converted (which is what
	// go vet objects to, rightly, for an address).
	hpc := p.console
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return err
	}

	si := &windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: attrs.List(),
	}
	cmdline, err := windows.UTF16PtrFromString(shell)
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

// reap waits for the program and closes the output pipe, so the pane's
// read loop sees EOF and the pane is marked dead.
func (p *winPTY) reap() {
	windows.WaitForSingleObject(p.proc, windows.INFINITE)
	close(p.exited)
	p.out.Close()
}

func (p *winPTY) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *winPTY) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *winPTY) Pid() int                    { return p.pid }
func (p *winPTY) Wait()                       { <-p.exited }

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

func (p *winPTY) release() {
	if p.console != 0 {
		windows.ClosePseudoConsole(p.console)
	}
	p.in.Close()
	p.out.Close()
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
