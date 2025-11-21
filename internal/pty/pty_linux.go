//go:build linux

package pty

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type linuxPty struct {
	ptmx *os.File
}

func New() (Pty, error) {
	return &linuxPty{}, nil
}

func (p *linuxPty) Start(cmd *exec.Cmd) error {
	var err error
	p.ptmx, err = pty.Start(cmd)
	return err
}

func (p *linuxPty) Close() error {
	return p.ptmx.Close()
}

func (p *linuxPty) Read(b []byte) (int, error) {
	return p.ptmx.Read(b)
}

func (p *linuxPty) Write(b []byte) (int, error) {
	return p.ptmx.Write(b)
}

func (p *linuxPty) SetSize(rows, cols uint16) error {
	winSize := &pty.Winsize{Rows: rows, Cols: cols}
	return pty.Setsize(p.ptmx, winSize)
}