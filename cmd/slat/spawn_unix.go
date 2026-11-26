//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// detachAttrs puts the daemon in its own session, so it survives the
// terminal that started it closing.
func detachAttrs() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// socketName is the daemon's socket file name, per user.
func socketName() string { return fmt.Sprintf("slat-%d.sock", os.Getuid()) }

// execAttach replaces the current client process with a fresh `slat attach`
// so no previous stdin reader goroutine lingers across session switches.
func execAttach(exe, sock string) error {
	return syscall.Exec(exe, []string{exe, "attach", sock}, os.Environ())
}
