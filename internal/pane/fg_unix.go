//go:build !windows

package pane

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// foreground returns the pid and command name of the process group that
// currently owns the pane's terminal — the program the user is talking
// to. It returns 0, "" when that can't be determined.
func foreground(fd uintptr) (int, string) {
	pgrp, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	if err != nil || pgrp <= 0 {
		return 0, ""
	}
	return pgrp, processName(pgrp)
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
