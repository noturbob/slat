//go:build !windows

package pane

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// unixPTY is a shell on a PTY pair. creack/pty puts it in its own session,
// so it has a controlling terminal and its own process group.
type unixPTY struct {
	cmd  *exec.Cmd
	f    *os.File
	done atomic.Bool
}

func startPTY(shell, dir string, rows, cols int) (ptyProcess, error) {
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Dir = dir
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{cmd: cmd, f: f}, nil
}

func (u *unixPTY) Read(b []byte) (int, error)  { return u.f.Read(b) }
func (u *unixPTY) Write(b []byte) (int, error) { return u.f.Write(b) }
func (u *unixPTY) Close() error                { return u.f.Close() }
func (u *unixPTY) Pid() int                    { return u.cmd.Process.Pid }

func (u *unixPTY) Resize(rows, cols int) error {
	return pty.Setsize(u.f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (u *unixPTY) Wait() {
	u.cmd.Wait()
	u.done.Store(true)
}

// Terminate sends SIGHUP to the whole process group, so jobs started from
// the shell go too. Anything that ignores it gets SIGKILL a moment later.
func (u *unixPTY) Terminate() {
	pid := u.cmd.Process.Pid
	syscall.Kill(-pid, syscall.SIGHUP)
	time.AfterFunc(2*time.Second, func() {
		if !u.done.Load() {
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

// Foreground asks the terminal which process group owns it: that is the
// program the user is typing to.
func (u *unixPTY) Foreground() (int, string) {
	pgrp, err := unix.IoctlGetInt(int(u.f.Fd()), unix.TIOCGPGRP)
	if err != nil || pgrp <= 0 {
		return 0, ""
	}
	return pgrp, processName(pgrp)
}

func (u *unixPTY) Cwd() string {
	dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", u.cmd.Process.Pid))
	if err != nil {
		return ""
	}
	return dir
}

func processName(pid int) string {
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	// macOS and the BSDs: no /proc, so ask ps. Callers cache this.
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name
}
