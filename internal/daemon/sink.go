package daemon

import (
	"io"
	"sync"
)

// sink is a thread-safe io.Writer that forwards to whichever client
// connection is currently attached, if any. Writes while nobody is
// attached are silently dropped: the shells keep running and their PTY
// output channels keep getting drained by the daemon's own forwarding
// loop, so nothing blocks — the terminal picture is just paused, exactly
// like a detached tmux session.
type sink struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *sink) Attach(w io.Writer) {
	s.mu.Lock()
	s.w = w
	s.mu.Unlock()
}

func (s *sink) Detach() {
	s.mu.Lock()
	s.w = nil
	s.mu.Unlock()
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	w := s.w
	s.mu.Unlock()
	if w == nil {
		return len(p), nil // discard while detached
	}
	if _, err := w.Write(p); err != nil {
		// Client connection died mid-write; detach so we go back to cheap
		// no-op writes until a new client attaches.
		s.Detach()
	}
	return len(p), nil
}