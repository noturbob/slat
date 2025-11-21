package pane

import (
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type Pane struct {
	ID     int
	Cmd    *exec.Cmd
	Pty    *os.File
	Rows   uint16
	Cols   uint16
	Output chan []byte // Buffered output channel
	IsDead bool
	mu     sync.Mutex
}

func New(id int, rows, cols uint16) (*Pane, error) {
	cmd := exec.Command("bash")
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
		Output: make(chan []byte, 4096), // Large buffer
	}

	go p.ReadLoop()
	return p, nil
}

func (p *Pane) ReadLoop() {
	defer close(p.Output)
	buf := make([]byte, 4096)
	for {
		n, err := p.Pty.Read(buf)
		if err != nil {
			return // Process died
		}
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			// Non-blocking send
			select {
			case p.Output <- data:
			default:
				// Drop frame if blocked
			}
		}
	}
}

func (p *Pane) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Rows = rows
	p.Cols = cols
	return pty.Setsize(p.Pty, &pty.Winsize{Rows: rows, Cols: cols})
}

func (p *Pane) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.IsDead {
		p.Pty.Close()
		if p.Cmd.Process != nil {
			p.Cmd.Process.Kill()
		}
		p.IsDead = true
	}
}