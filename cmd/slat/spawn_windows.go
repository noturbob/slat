package main

import (
	"os"
	"syscall"
)

// detachAttrs starts the daemon with no console of its own, so it outlives
// the window that started it — Windows' equivalent of a new session.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
		HideWindow:    true,
	}
}

// socketName is the daemon's socket file name. Windows has no uid, and
// the temporary directory is already per-user.
func socketName() string {
	name := os.Getenv("USERNAME")
	if name == "" {
		name = "user"
	}
	return "slat-" + name + ".sock"
}
