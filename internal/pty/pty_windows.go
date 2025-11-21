//go:build windows

package pty

import (
	"errors"
	"os/exec"
)

type windowsPty struct{}

func New() (Pty, error) {
	return nil, errors.New("slat is not yet supported on Windows")
}

func (p *windowsPty) Start(cmd *exec.Cmd) error {
	return errors.New("unsupported on Windows")
}

func (p *windowsPty) Close() error {
	return errors.New("unsupported on Windows")
}

func (p *windowsPty) Read(b []byte) (int, error) {
	return 0, errors.New("unsupported on Windows")
}

func (p *windowsPty) Write(b []byte) (int, error) {
	return 0, errors.New("unsupported on Windows")
}

func (p *windowsPty) SetSize(rows, cols uint16) error {
	return errors.New("unsupported on Windows")
}