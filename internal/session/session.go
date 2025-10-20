package session

import "github.com/noturbob/slat/internal/pane"

// Session represents a collection of windows and panes.
type Session struct {
	ID            int
	ActivePaneID  int
	Panes         []*pane.Pane
}

// New creates a new session.
func New() *Session {
	return &Session{
		Panes: make([]*pane.Pane, 0),
	}
}