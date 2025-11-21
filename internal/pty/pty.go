package pty

import (
	"io"
	"os/exec"
)

type Pty interface {
	Start(cmd *exec.Cmd) error
	Close() error
	SetSize(rows, cols uint16) error
	io.Reader
	io.Writer
}