package pane

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// SplitDirection indicates how a layout node is split.
type SplitDirection int

const (
	SplitNone       SplitDirection = iota
	SplitHorizontal                // top/bottom
	SplitVertical                  // left/right
)

// Pane represents a single PTY-backed terminal pane.
type Pane struct {
	ID     int
	Cmd    *exec.Cmd
	Pty    *os.File
	Rows   uint16
	Cols   uint16
	Row    int // position in the terminal grid (1-based)
	Col    int
	Output chan []byte
	IsDead bool
	Zoomed bool
	mu     sync.Mutex
}

// New creates and starts a new pane with the given shell.
func New(id int, rows, cols uint16, shell string) (*Pane, error) {
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	pt, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, err
	}

	p := &Pane{
		ID:     id,
		Cmd:    cmd,
		Pty:    pt,
		Rows:   rows,
		Cols:   cols,
		Output: make(chan []byte, 8192),
	}

	go p.readLoop()
	go p.waitLoop()
	return p, nil
}

func (p *Pane) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := p.Pty.Read(buf)
		if err != nil {
			p.mu.Lock()
			p.IsDead = true
			p.mu.Unlock()
			return
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case p.Output <- data:
			default:
				// Drop oldest if channel full, then push new
				select {
				case <-p.Output:
				default:
				}
				select {
				case p.Output <- data:
				default:
				}
			}
		}
	}
}

func (p *Pane) waitLoop() {
	if p.Cmd.Process != nil {
		_ = p.Cmd.Wait()
	}
	p.mu.Lock()
	p.IsDead = true
	p.mu.Unlock()
}

// Write sends input data to the pane's PTY.
func (p *Pane) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.IsDead {
		return 0, os.ErrClosed
	}
	return p.Pty.Write(data)
}

// Resize changes the PTY window size.
func (p *Pane) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.IsDead {
		return nil
	}
	p.Rows = rows
	p.Cols = cols
	return pty.Setsize(p.Pty, &pty.Winsize{Rows: rows, Cols: cols})
}

// SetPosition sets the pane's top-left position in the terminal.
func (p *Pane) SetPosition(row, col int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Row = row
	p.Col = col
}

// Dead returns whether the pane's process has exited.
func (p *Pane) Dead() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.IsDead
}

// Close terminates the pane's process and closes the PTY.
func (p *Pane) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.IsDead {
		p.IsDead = true
		p.Pty.Close()
		if p.Cmd.Process != nil {
			_ = p.Cmd.Process.Signal(os.Interrupt)
			_ = p.Cmd.Process.Kill()
		}
	}
}
