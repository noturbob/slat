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
