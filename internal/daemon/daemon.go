package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/control"
	"github.com/noturbob/slat/internal/proto"
	"github.com/noturbob/slat/internal/session"
)

// Server is the slat background daemon. It owns the single running
// session — every workspace, tab, pane, and shell — and lets terminal
// clients attach and detach from it without disturbing anything running.
type Server struct {
	app      *app.App
	sink     *sink
	sockPath string

	stop     chan struct{}
	stopOnce sync.Once

	mu     sync.Mutex
	client net.Conn // currently attached client, if any
}

// New constructs a daemon bound to the given Unix socket path. It does not
// start the session or listen yet — call Run for that.
func New(cfg *config.Config, sockPath string) (*Server, error) {
	// Panes inherit this, so `slat` run inside a pane can refuse to nest.
	if err := os.Setenv("SLAT", sockPath); err != nil {
		return nil, err
	}
	a, err := app.New(cfg)
	if err != nil {
		return nil, err
	}
	s := &sink{}
	a.SetOutput(s)
	a.SetSessionLister(func() []app.SessionSummary {
		dir := filepath.Dir(sockPath)
		base := filepath.Base(sockPath)
		prefix := "slat-"
		if i := strings.LastIndexByte(base, '-'); i > 0 {
			prefix = base[:i]
		}
		// Skip sockPath itself: openSessionPicker holds a.mu, so dialing our
		// own socket would deadlock until the control timeout.
		return control.ListSessions([]string{dir, os.Getenv("XDG_RUNTIME_DIR")}, prefix, sockPath)
	})
	a.SetGlobalTreeLister(func() []app.RemoteTreeNode {
		dir := filepath.Dir(sockPath)
		base := filepath.Base(sockPath)
		prefix := "slat-"
		if i := strings.LastIndexByte(base, '-'); i > 0 {
			prefix = base[:i]
		}
		return control.ListGlobalTree([]string{dir, os.Getenv("XDG_RUNTIME_DIR")}, prefix, sockPath)
	})
	a.SetRemoteFocuser(control.FocusRemote)
	return &Server{app: a, sink: s, sockPath: sockPath, stop: make(chan struct{})}, nil
}

// SetPreset configures a preconfigured session to load when Run starts.
func (s *Server) SetPreset(p session.Preset) {
	s.app.SetPreset(p)
}

// Run starts the session immediately (so shells are alive even before any
// client attaches, like `tmux new -d`), listens on the Unix socket, and
// serves clients until the session ends or the daemon is signaled to stop.
func (s *Server) Run() error {
	// Two clients starting at once can race to spawn two daemons; the
	// loser must not delete the winner's socket and orphan its session.
	if c, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext(context.Background(), "unix", s.sockPath); err == nil {
		if cErr := c.Close(); cErr != nil {
			log.Printf("slat: close probe socket: %v", cErr)
		}
		return fmt.Errorf("a slat daemon is already running on %s", s.sockPath)
	}
	if err := os.Remove(s.sockPath); err != nil && !errors.Is(err, os.ErrNotExist) { // stale socket from a crashed daemon
		return fmt.Errorf("remove stale socket %s: %w", s.sockPath, err)
	}

	ln, err := listenPrivate(s.sockPath) // only this user may attach
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.sockPath, err)
	}
	if startErr := s.app.Start(80, 24); startErr != nil {
		if cErr := ln.Close(); cErr != nil {
			log.Printf("slat: close listener: %v", cErr)
		}
		return fmt.Errorf("failed to start session: %w", startErr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	const maxConnections = 64
	connSem := make(chan struct{}, maxConnections)

	go func() {
		for {
			c, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			select {
			case connSem <- struct{}{}:
				go func() {
					defer func() { <-connSem }()
					s.serve(c)
				}()
			default:
				if closeErr := c.Close(); closeErr != nil {
					log.Printf("slat: close rejected client: %v", closeErr)
				}
			}
		}
	}()

	for {
		select {
		case <-s.app.DetachRequested():
			s.kick()
			continue
		case <-sigCh:
		case <-s.stop:
		case <-s.app.Done():
		}
		// Stop accepting and remove the socket before dropping the client,
		// so the client can tell "session ended" from "detached".
		if closeErr := ln.Close(); closeErr != nil {
			log.Printf("slat: close listener: %v", closeErr)
		}
		if rmErr := os.Remove(s.sockPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			log.Printf("slat: remove socket %s: %v", s.sockPath, rmErr)
		}
		s.kick()
		s.app.Shutdown()
		return nil
	}
}

// Stop ends the session and makes Run return.
func (s *Server) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// kick disconnects the attached client, if any.
func (s *Server) kick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		if err := s.client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("slat: close client: %v", err)
		}
		s.client = nil
		s.sink.Attach(nil)
		s.app.Detach()
	}
}

// serve attaches conn as the active client and feeds its input into the
// session until it disconnects. A new client replaces the current one,
// like `tmux attach -d`.
func (s *Server) serve(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("slat: close connection: %v", err)
		}
	}()

	// Liveness probes connect and hang up without a hello; they must not
	// kick the attached client.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	hello, err := proto.ReadFrame(conn)
	if err != nil {
		return
	}
	if hello.Type == proto.TypeControl {
		// The slat CLI: answer the command and hang up, leaving the
		// attached client alone. `wait` can block for a long time.
		if deadlineErr := conn.SetReadDeadline(time.Time{}); deadlineErr != nil {
			return
		}
		s.serveControl(conn, hello.Payload)
		return
	}
	if hello.Type != proto.TypeHello {
		return
	}
	if deadlineErr := conn.SetReadDeadline(time.Time{}); deadlineErr != nil {
		return
	}
	cols, rows := proto.DecodeSize(hello.Payload)

	s.mu.Lock()
	if s.client != nil {
		if closeErr := s.client.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			log.Printf("slat: close previous client: %v", closeErr)
		}
	}
	s.client = conn
	s.sink.Attach(conn)
	s.mu.Unlock()
	s.app.Attach(cols, rows)

	for {
		f, readErr := proto.ReadFrame(conn)
		if readErr != nil {
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
