package app

import (
	"os"
	"os/exec"
	"strings"
)

// hookShell runs a configured hook through the console shell.
func hookShell(line string) *exec.Cmd {
	sh := os.Getenv("COMSPEC")
	if sh == "" {
		sh = "cmd.exe"
	}
	return exec.Command(sh, "/c", line)
}

// quote makes s one cmd.exe argument. cmd.exe has no quoting that is safe
// for every byte, so the characters it would act on are dropped rather
// than escaped — a pane's output must not become part of the command. Use
// $SLAT_LINE from the environment if you need the text exactly.
func quote(s string) string {
	return `"` + strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`"&|<>^%()!`, r) {
			return -1
		}
		return r
	}, s) + `"`
}
