package client

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/noturbob/slat/internal/proto"
)

// Run connects to the daemon at sockPath and attaches an interactive
// session: local raw mode, stdin -> daemon, daemon output -> stdout.
// It returns once the daemon detaches or closes the connection.
func Run(sockPath string) error {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("connect to daemon: %w", err)
	}
	defer conn.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to enter raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		cols, rows = 80, 24
	}
	if err := proto.WriteFrame(conn, proto.TypeHello, proto.EncodeSize(cols, rows)); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	done := make(chan struct{})
	var closeOnce chanCloser

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-sigCh:
				c, r, err := term.GetSize(int(os.Stdin.Fd()))
				if err == nil {
					_ = proto.WriteFrame(conn, proto.TypeResize, proto.EncodeSize(c, r))
				}
			case <-done:
				return
			}
		}
	}()

	// stdin -> daemon
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := os.Stdin.Read(buf)
			if n > 0 {
				if werr := proto.WriteFrame(conn, proto.TypeInput, buf[:n]); werr != nil {
					closeOnce.Close(done)
					return
				}
			}
			if rerr != nil {
				closeOnce.Close(done)
				return
			}
		}
	}()

	// daemon -> stdout: raw passthrough, the daemon already sends
	// fully-rendered ANSI output.
	buf := make([]byte, 32768)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	closeOnce.Close(done)

	term.Restore(int(os.Stdin.Fd()), oldState)
	fmt.Println("slat: detached")
	return nil
}

// chanCloser closes a channel exactly once, even if Close is called from
// multiple goroutines concurrently (stdin reader and the main read loop
// can both hit an error/EOF at roughly the same time on detach).
type chanCloser struct {
	done bool
}

func (c *chanCloser) Close(ch chan struct{}) {
	if !c.done {
		c.done = true
		close(ch)
	}
}