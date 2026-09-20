//go:build !windows

package app

import (
	"os/exec"
	"strings"
)

// hookShell runs a configured hook through /bin/sh.
func hookShell(line string) *exec.Cmd { return exec.Command("/bin/sh", "-c", line) }

// quote makes s a single /bin/sh word, whatever it contains.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
