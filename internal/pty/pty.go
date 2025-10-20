package pty

import (
	"io"
	"os/exec"
)

// Pty represents the common interface for a pseudo-terminal.
type Pty interface {
	Start(cmd *exec.Cmd) error
	Close() error
	// SetSize updates the pseudo-terminal's window size.
	SetSize(rows, cols uint16) error
	io.Reader
	io.Writer
}