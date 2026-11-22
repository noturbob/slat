package client

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/term"

	"github.com/noturbob/slat/internal/proto"
)

const (
	// Entered on attach: alternate screen, so the user's shell scrollback
	// is untouched and comes back intact on exit.
	enterScreen = "\x1b[?1049h"
	// Undo anything the session may have left set on the terminal.
	// Mouse reporting is turned off here too: leaving it on would cost the
	// user their terminal's own selection after slat exits.
	leaveScreen = "\x1b[0m\x1b[?25h\x1b[0 q\x1b[?1l\x1b[?2004l\x1b[?1002l\x1b[?1006l\x1b[r\x1b[?1049l"
)

// Run attaches this terminal to the daemon at sockPath and returns once the
// client is detached or the session ends. The terminal is always restored.
func Run(sockPath string) (detached bool, err error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return false, fmt.Errorf("connect to daemon: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			log.Printf("slat: close client socket: %v", closeErr)
		}
	}()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false, fmt.Errorf("stdin is not a terminal: %w", err)
	}
	restoreOutput := prepareOutput()
	if _, enterErr := os.Stdout.WriteString(enterScreen); enterErr != nil {
		restoreOutput()
		if rErr := term.Restore(fd, oldState); rErr != nil {
			log.Printf("slat: restore terminal: %v", rErr)
		}
		return false, fmt.Errorf("enter alternate screen: %w", enterErr)
	}
	defer func() {
		if _, leaveErr := os.Stdout.WriteString(leaveScreen); leaveErr != nil {
			log.Printf("slat: leave alternate screen: %v", leaveErr)
		}
		restoreOutput()
		if restoreErr := term.Restore(fd, oldState); restoreErr != nil {
			log.Printf("slat: restore terminal: %v", restoreErr)
		}
	}()

	// Frames come from two goroutines (stdin and the resize watcher); a
	// frame's header and payload must not interleave with another frame's.
	var wmu sync.Mutex
	send := func(t proto.FrameType, payload []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return proto.WriteFrame(conn, t, payload)
	}
	size := func() []byte {
		cols, rows, sizeErr := term.GetSize(fd)
		if sizeErr != nil {
			cols, rows = 80, 24
		}
		return proto.EncodeSize(cols, rows)
	}

	if helloErr := send(proto.TypeHello, size()); helloErr != nil {
		return false, fmt.Errorf("handshake failed: %w", helloErr)
	}

	stopWatching := watchTerminal(fd,
		func() {
			if resizeErr := send(proto.TypeResize, size()); resizeErr != nil {
				log.Printf("slat: send resize: %v", resizeErr)
			}
		},
		// Unblocks the read loop below; the defers restore the terminal.
		func() {
			if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				log.Printf("slat: close client socket: %v", closeErr)
			}
		})
	defer stopWatching()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, stdinErr := os.Stdin.Read(buf)
			if n > 0 && send(proto.TypeInput, buf[:n]) != nil {
				return
			}
			if stdinErr != nil {
				if cErr := conn.Close(); cErr != nil && !errors.Is(cErr, net.ErrClosed) {
					log.Printf("slat: close client socket: %v", cErr)
				}
				return
			}
		}
	}()

	// daemon -> terminal: already fully rendered output.
	buf := make([]byte, 64*1024)
	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			if _, wErr := os.Stdout.Write(buf[:n]); wErr != nil {
				log.Printf("slat: write stdout: %v", wErr)
				break
			}
		}
		if readErr != nil {
			break
		}
	}
	// The daemon removes its socket before hanging up when the session
	// ends, so a socket that still answers means we were only detached.
	c, err := net.Dial("unix", sockPath)
	if err == nil {
		if cErr := c.Close(); cErr != nil {
			log.Printf("slat: close probe socket: %v", cErr)
		}
	}
	return err == nil, nil
}
