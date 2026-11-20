//go:build !windows

package app

import (
	"context"
	"os/exec"
	"strings"
)

// hookShell runs a configured hook through /bin/sh.
func hookShell(line string) *exec.Cmd { return exec.Command("/bin/sh", "-c", line) }

// hookShellContext runs a configured command through /bin/sh bound to ctx.
func hookShellContext(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", line)
}

// quote makes s a single /bin/sh word, whatever it contains.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
