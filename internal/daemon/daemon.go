package daemon

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/proto"
)

// Server is the slat background daemon. It owns the single running
// session — every workspace, tab, pane, and shell — and lets terminal
// clients attach and detach from it without disturbing anything running.
type Server struct {
	app      *app.App
	sink     *sink
	sockPath string

	mu     sync.Mutex
	client net.Conn // currently attached client, if any
}

// New constructs a daemon bound to the given Unix socket path. It does not
// start the session or listen yet — call Run for that.
func New(cfg *config.Config, sockPath string) (*Server, error) {
	// Panes inherit this, so `slat` run inside a pane can refuse to nest.
	os.Setenv("SLAT", sockPath)
	a, err := app.New(cfg)
	if err != nil {
		return nil, err
	}
	s := &sink{}
	a.SetOutput(s)
	return &Server{app: a, sink: s, sockPath: sockPath}, nil
}

// Run starts the session immediately (so shells are alive even before any
// client attaches, like `tmux new -d`), listens on the Unix socket, and
// serves clients until the session ends or the daemon is signaled to stop.
func (s *Server) Run() error {
	// Two clients starting at once can race to spawn two daemons; the
	// loser must not delete the winner's socket and orphan its session.
	if c, err := net.DialTimeout("unix", s.sockPath, 200*time.Millisecond); err == nil {
		c.Close()
		return fmt.Errorf("a slat daemon is already running on %s", s.sockPath)
	}
	os.Remove(s.sockPath) // stale socket from a crashed daemon

	old := syscall.Umask(0o077) // socket is created 0600: only we may attach
	ln, err := net.Listen("unix", s.sockPath)
	syscall.Umask(old)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.sockPath, err)
	}
	if err := s.app.Start(80, 24); err != nil {
		ln.Close()
		return fmt.Errorf("failed to start session: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()

	for {
		select {
		case <-s.app.DetachRequested():
			s.kick()
			continue
		case <-sigCh:
		case <-s.app.Done():
		}
		// Stop accepting and remove the socket before dropping the client,
		// so the client can tell "session ended" from "detached".
		ln.Close()
		os.Remove(s.sockPath)
		s.kick()
		s.app.Shutdown()
		return nil
	}
}

// kick disconnects the attached client, if any.
func (s *Server) kick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
		s.sink.Attach(nil)
		s.app.Detach()
	}
}

// serve attaches conn as the active client and feeds its input into the
// session until it disconnects. A new client replaces the current one,
// like `tmux attach -d`.
func (s *Server) serve(conn net.Conn) {
	defer conn.Close()

	// Liveness probes connect and hang up without a hello; they must not
	// kick the attached client.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	hello, err := proto.ReadFrame(conn)
	if err != nil || hello.Type != proto.TypeHello {
		return
	}
	conn.SetReadDeadline(time.Time{})
	cols, rows := proto.DecodeSize(hello.Payload)

	s.mu.Lock()
	if s.client != nil {
		s.client.Close()
	}
	s.client = conn
	s.sink.Attach(conn)
	s.mu.Unlock()
	s.app.Attach(cols, rows)

	for {
		f, err := proto.ReadFrame(conn)
		if err != nil {
			break
		}
		switch f.Type {
		case proto.TypeInput:
			s.app.FeedInput(f.Payload)
		case proto.TypeResize:
			s.app.HandleResize(proto.DecodeSize(f.Payload))
		}
	}

	s.mu.Lock()
	if s.client == conn {
		s.client = nil
		s.sink.Attach(nil)
		s.app.Detach()
	}
	s.mu.Unlock()
}
