//go:build !windows

package client

import (
	"os"
	"os/signal"
	"syscall"
)

// watchTerminal calls onResize when the terminal's size changes and onQuit
// when the system asks this client to go away. The returned function stops
// watching.
func watchTerminal(fd int, onResize, onQuit func()) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range ch {
			if sig == syscall.SIGWINCH {
				onResize()
				continue
			}
			onQuit()
			return
		}
	}()
	return func() { signal.Stop(ch) }
}

// prepareOutput is a no-op: a Unix terminal already understands the escape
// sequences slat writes.
func prepareOutput() func() { return func() {} }
