package daemon

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/proto"
)

// Server is the slat background daemon. It owns the single running
// session — every workspace, tab, pane, and shell — and lets terminal
// clients attach and detach from it without disturbing anything running.
type Server struct {
	cfg      *config.Config
	app      *app.App
	sink     *sink
	sockPath string

	mu     sync.Mutex
	client net.Conn // currently attached client, if any
}

// New constructs a daemon bound to the given Unix socket path. It does not
// start the session or listen yet — call Run for that.
func New(cfg *config.Config, sockPath string) (*Server, error) {
	a, err := app.New(cfg)
	if err != nil {
		return nil, err
	}
	s := &sink{}
	a.SetOutput(s)
	return &Server{cfg: cfg, app: a, sink: s, sockPath: sockPath}, nil
}

// Run starts the session immediately (so shells are alive even before any
// client attaches, like `tmux new -d`), listens on the Unix socket, and
// serves clients until the session ends or the daemon is signaled to stop.
func (s *Server) Run() error {
	if err := s.app.Start(80, 24); err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	_ = os.Remove(s.sockPath)
	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.sockPath, err)
	}
	defer os.Remove(s.sockPath)
	defer ln.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	acceptCh := make(chan net.Conn)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			acceptCh <- c
		}
	}()

	for {
		select {
		case <-sigCh:
			s.app.Shutdown()
			return nil
		case <-s.app.Done():
			// All panes/tabs/workspaces closed (e.g. via 'q' or the last
			// shell exiting) — the session is over, so the daemon exits.
			s.app.Shutdown()
			return nil
		case conn := <-acceptCh:
			s.handleClient(conn)
		}
	}
}

// handleClient attaches the given connection as the active client, streams
// its input into the session, and returns once it detaches, disconnects,
// or the session ends. Only one client is attached at a time — connecting
// while another client is attached kicks the previous one, same as tmux
// attaching to an already-attached session.
func (s *Server) handleClient(conn net.Conn) {
	s.mu.Lock()
	if s.client != nil {
		s.client.Close()
	}
	s.client = conn
	s.mu.Unlock()

	hello, err := proto.ReadFrame(conn)
	if err != nil || hello.Type != proto.TypeHello {
		conn.Close()
		return
	}
	cols, rows := proto.DecodeSize(hello.Payload)

	s.sink.Attach(conn)
	s.app.HandleResize(cols, rows) // also triggers a full redraw for the new client

	frameCh := make(chan proto.Frame)
	errCh := make(chan error, 1)
	go func() {
		for {
			f, err := proto.ReadFrame(conn)
			if err != nil {
				errCh <- err
				return
			}
			frameCh <- f
		}
	}()

	for {
		select {
		case f := <-frameCh:
			switch f.Type {
			case proto.TypeInput:
				s.app.FeedInput(f.Payload)
			case proto.TypeResize:
				c, r := proto.DecodeSize(f.Payload)
				s.app.HandleResize(c, r)
			}
		case <-s.app.DetachRequested():
			conn.Close()
			s.clearClient(conn)
			return
		case <-s.app.Done():
			conn.Close()
			s.clearClient(conn)
			return
		case <-errCh:
			// Connection dropped (terminal closed, network hiccup, etc.)
			// — treated exactly like an explicit detach: session lives on.
			s.clearClient(conn)
			return
		}
	}
}

func (s *Server) clearClient(conn net.Conn) {
	s.mu.Lock()
	if s.client == conn {
		s.client = nil
	}
	s.mu.Unlock()
	s.sink.Detach()
}