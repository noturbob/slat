package client

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// resizePoll is how often the console's size is checked. Windows has no
// SIGWINCH, so this is the only way to notice a resize.
const resizePoll = 250 * time.Millisecond

func watchTerminal(fd int, onResize, onQuit func()) func() {
	done := make(chan struct{})
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer signal.Stop(ch)
		cols, rows, _ := term.GetSize(fd)
		tick := time.NewTicker(resizePoll)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-ch:
				onQuit()
				return
			case <-tick.C:
				if c, r, err := term.GetSize(fd); err == nil && (c != cols || r != rows) {
					cols, rows = c, r
					onResize()
				}
			}
		}
	}()
	return func() { close(done) }
}

// prepareOutput turns on the console's escape-sequence handling, which is
// off by default, and restores it afterwards. Without it slat's output
// appears as literal escape codes.
func prepareOutput() func() {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return func() {}
	}
	want := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.DISABLE_NEWLINE_AUTO_RETURN
	if err := windows.SetConsoleMode(h, want); err != nil {
		// An old console can't do VT at all; there is nothing to restore.
		return func() {}
	}
	return func() { windows.SetConsoleMode(h, mode) }
}
