package daemon

import (
	"net"
	"sync"
	"time"
)

// writeTimeout bounds how long a stalled client (suspended terminal, dead
// network) can block rendering before it's dropped.
const writeTimeout = 5 * time.Second

// sink is the session's output: it forwards to whichever client is
// attached and discards output while none is. Nothing is lost by
// discarding, since every attach repaints the screen from the panes'
// emulators.
type sink struct {
	mu   sync.Mutex
	conn net.Conn
}

// Attach sets the client to write to; nil detaches.
func (s *sink) Attach(c net.Conn) {
	s.mu.Lock()
	s.conn = c
	s.mu.Unlock()
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	c := s.conn
	s.mu.Unlock()
	if c == nil {
		return len(p), nil
	}
	c.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := c.Write(p); err != nil {
		// Closing makes the client's serve loop exit and clean up.
		c.Close()
	}
	return len(p), nil
}
