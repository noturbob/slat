package pane

import "os/exec"

// Pane represents a single terminal pane within a window.
type Pane struct {
	ID      int
	Cmd     *exec.Cmd
	IsActive bool
}

// New creates a new Pane.
func New() *Pane {
	return &Pane{
		IsActive: true,
	}
}