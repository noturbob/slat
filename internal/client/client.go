package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/term"

	"github.com/noturbob/slat/internal/proto"
)

const (
	// Entered on attach: alternate screen, so the user's shell scrollback
	// is untouched and comes back intact on exit.
	enterScreen = "\x1b[?1049h"
	// Undo anything the session may have left set on the terminal.
	leaveScreen = "\x1b[0m\x1b[?25h\x1b[0 q\x1b[?1l\x1b[?2004l\x1b[r\x1b[?1049l"
)

// Run attaches this terminal to the daemon at sockPath and returns once the
// client is detached or the session ends. The terminal is always restored.
func Run(sockPath string) (detached bool, err error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false, fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, fmt.Errorf("stdin is not a terminal: %w", err)
	}
	os.Stdout.WriteString(enterScreen)
	defer func() {
		os.Stdout.WriteString(leaveScreen)
		term.Restore(fd, oldState)
	}()

	// Frames come from two goroutines (stdin and SIGWINCH); a frame's header
	// and payload must not interleave with another frame's.
	var wmu sync.Mutex
	send := func(t proto.FrameType, payload []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return proto.WriteFrame(conn, t, payload)
	}
	size := func() []byte {
		cols, rows, err := term.GetSize(fd)
		if err != nil {
			cols, rows = 80, 24
		}
		return proto.EncodeSize(cols, rows)
	}

	if err := send(proto.TypeHello, size()); err != nil {
		return false, fmt.Errorf("handshake failed: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			if sig != syscall.SIGWINCH {
				conn.Close() // unblocks the read loop below; defers restore the terminal
				return
			}
			send(proto.TypeResize, size())
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && send(proto.TypeInput, buf[:n]) != nil {
				return
			}
			if err != nil {
				conn.Close()
				return
			}
		}
	}()

	// daemon -> terminal: already fully rendered output.
	buf := make([]byte, 64*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	// The daemon removes its socket before hanging up when the session
	// ends, so a socket that still answers means we were only detached.
	c, err := net.Dial("unix", sockPath)
	if err == nil {
		c.Close()
	}
	return err == nil, nil
}
