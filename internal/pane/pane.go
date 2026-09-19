package pane

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/noturbob/slat/internal/vt"
)

// SplitDirection indicates how a layout node is split.
type SplitDirection int

const (
	SplitNone       SplitDirection = iota
	SplitHorizontal                // top/bottom
	SplitVertical                  // left/right
)

// Pane is a shell running on a PTY, with a terminal emulator that keeps
// its screen. slat never streams PTY bytes to the real terminal; it paints
// each pane from its emulator, so a pane's contents survive splits, closes,
// tab switches and reattaches.
type Pane struct {
	ID int

	cmd *exec.Cmd
	pty *os.File

	mu       sync.Mutex
	term     *vt.Terminal
	row, col int // top-left position on screen, 0-based
	dead     bool
	exited   bool // process reaped

	closeOnce sync.Once
	onChange  func()
}

// New starts shell on a rows x cols PTY in directory dir ("" = inherit).
// onChange is called from another goroutine whenever the pane's screen
// changes or its process exits.
func New(id int, rows, cols int, shell, dir string, onChange func()) (*Pane, error) {
	rows, cols = max(rows, 1), max(cols, 1)
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Dir = dir
	pt, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	if onChange == nil {
		onChange = func() {}
	}
	p := &Pane{ID: id, cmd: cmd, pty: pt, term: vt.New(cols, rows), onChange: onChange}
	go p.readLoop()
	go p.waitLoop()
	return p, nil
}

func (p *Pane) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			p.mu.Lock()
			p.term.Write(buf[:n])
			replies := p.term.Replies()
			p.mu.Unlock()
			if len(replies) > 0 {
				p.pty.Write(replies) // answers to cursor-position/device queries
			}
			p.onChange()
		}
		if err != nil {
			p.markDead()
			return
		}
	}
}

func (p *Pane) waitLoop() {
	p.cmd.Wait()
	p.mu.Lock()
	p.exited = true
	p.mu.Unlock()
	p.markDead()
}

// markDead flags the pane as finished and releases its PTY.
// Safe to call any number of times from any goroutine.
func (p *Pane) markDead() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.dead = true
		p.mu.Unlock()
		p.pty.Close()
		p.onChange()
	})
}

// Close ends the pane's program the way closing a terminal window does:
// SIGHUP to its whole process group, so jobs started from the shell go too.
// Anything that ignores SIGHUP gets SIGKILL a moment later.
func (p *Pane) Close() {
	if p.Dead() {
		return
	}
	pid := p.cmd.Process.Pid
	syscall.Kill(-pid, syscall.SIGHUP)
	p.markDead()
	time.AfterFunc(2*time.Second, func() {
		p.mu.Lock()
		exited := p.exited
		p.mu.Unlock()
		if !exited {
			syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

// Write sends input to the pane's program.
func (p *Pane) Write(data []byte) (int, error) {
	if p.Dead() {
		return 0, os.ErrClosed
	}
	return p.pty.Write(data)
}

// Dead reports whether the pane's program has exited or been closed.
func (p *Pane) Dead() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dead
}

// SetRect moves and resizes the pane. The program only gets SIGWINCH when
// the size actually changes, so calling this on every frame is cheap.
func (p *Pane) SetRect(row, col, rows, cols int) {
	rows, cols = max(rows, 1), max(cols, 1)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.row, p.col = row, col
	if c, r := p.term.Size(); p.dead || (c == cols && r == rows) {
		return
	}
	p.term.Resize(cols, rows)
	pty.Setsize(p.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Rect returns the pane's position (0-based) and size.
func (p *Pane) Rect() (row, col, rows, cols int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows = p.term.Size()
	return p.row, p.col, rows, cols
}

// Draw copies the pane's screen into screen (rows of cells) at the pane's
// position, clipped to its bounds.
func (p *Pane) Draw(screen [][]vt.Cell) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cols, rows := p.term.Size()
	for y := 0; y < rows && p.row+y < len(screen); y++ {
		if p.row+y < 0 || p.col >= len(screen[p.row+y]) {
			continue
		}
		dst := screen[p.row+y][p.col:]
		copy(dst[:min(cols, len(dst))], p.term.Line(y))
		if n := len(dst); n < cols && dst[n-1].Wide == vt.WideHead {
			dst[n-1] = vt.Cell{Style: dst[n-1].Style} // clipped wide char
		}
	}
}

// Cursor returns where the pane's cursor is (relative to the pane) and
// how it should look.
func (p *Pane) Cursor() (x, y int, visible bool, style int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	x, y = p.term.Cursor()
	return x, y, p.term.CursorVisible() && !p.dead, p.term.CursorStyle()
}

// Modes reports the input modes the program has enabled: application
// cursor keys (DECCKM) and bracketed paste.
func (p *Pane) Modes() (appCursor, bracketedPaste bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term.AppCursor(), p.term.BracketedPaste()
}

// Cwd returns the working directory of the pane's shell, or "" if the
// platform doesn't expose it (only Linux's /proc does).
func (p *Pane) Cwd() string {
	dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", p.cmd.Process.Pid))
	if err != nil {
		return ""
	}
	return dir
}
