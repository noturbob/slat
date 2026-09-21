package pane

import (
	"os"
	"strings"
	"sync"
	"time"

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

	proc ptyProcess

	mu       sync.Mutex
	term     *vt.Terminal
	row, col int // top-left position on screen, 0-based
	dead     bool
	lastOut  time.Time

	fgPID  int // foreground process, cached: on macOS this costs a ps call
	fgName string
	fgAt   time.Time

	closeOnce sync.Once
	onChange  func()
}

// New starts shell on a rows x cols PTY in directory dir ("" = inherit),
// keeping scrollback lines of history. onChange is called from another
// goroutine whenever the pane's screen changes or its process exits.
func New(id int, rows, cols int, shell, dir string, scrollback int, onChange func()) (*Pane, error) {
	rows, cols = max(rows, 1), max(cols, 1)
	proc, err := startPTY(shell, dir, rows, cols)
	if err != nil {
		return nil, err
	}
	if onChange == nil {
		onChange = func() {}
	}
	// Counted as output from the start: a pane that hasn't printed its
	// first prompt yet is starting up, not idle.
	p := &Pane{ID: id, proc: proc, term: vt.New(cols, rows), onChange: onChange, lastOut: time.Now()}
	p.term.SetScrollback(scrollback)
	go p.readLoop()
	go p.waitLoop()
	return p, nil
}

func (p *Pane) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, err := p.proc.Read(buf)
		if n > 0 {
			p.mu.Lock()
			p.term.Write(buf[:n])
			replies := p.term.Replies()
			p.lastOut = time.Now()
			// Output means something started or finished, so the cached
			// foreground process is the stalest thing in this struct.
			p.fgAt = time.Time{}
			p.mu.Unlock()
			if len(replies) > 0 {
				p.proc.Write(replies) // answers to cursor-position/device queries
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
	p.proc.Wait()
	p.markDead()
}

// markDead flags the pane as finished and releases its PTY.
// Safe to call any number of times from any goroutine.
func (p *Pane) markDead() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.dead = true
		p.mu.Unlock()
		p.proc.Close()
		p.onChange()
	})
}

// Close ends the pane's program the way closing a terminal window does,
// taking anything it started with it.
func (p *Pane) Close() {
	if p.Dead() {
		return
	}
	p.proc.Terminate()
	p.markDead()
}

// Write sends input to the pane's program.
func (p *Pane) Write(data []byte) (int, error) {
	if p.Dead() {
		return 0, os.ErrClosed
	}
	return p.proc.Write(data)
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
	p.proc.Resize(rows, cols)
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
	p.draw(screen, p.row, p.col, p.term.Line)
}

// DrawAt is Draw with the screen painted at another position, for animating
// a pane on its way somewhere. The pane's own size and position are
// untouched, so nothing it runs is resized or reflowed.
func (p *Pane) DrawAt(screen [][]vt.Cell, row, col int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.draw(screen, row, col, p.term.Line)
}

// DrawHistory is Draw for a view scrolled back through history: the view's
// first row is the line with absolute index top (see History).
func (p *Pane) DrawHistory(screen [][]vt.Cell, top int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.draw(screen, p.row, p.col, func(y int) []vt.Cell { return p.term.LineAt(top + y) })
}

func (p *Pane) draw(screen [][]vt.Cell, row, col int, line func(y int) []vt.Cell) {
	cols, rows := p.term.Size()
	for y := 0; y < rows && row+y < len(screen); y++ {
		if row+y < 0 || col < 0 || col >= len(screen[row+y]) {
			continue
		}
		dst := screen[row+y][col:]
		dst = dst[:min(cols, len(dst))]
		n := copy(dst, line(y)) // history lines may be shorter or longer
		if n > 0 && dst[n-1].Wide == vt.WideHead {
			dst[n-1] = vt.Cell{Style: dst[n-1].Style} // clipped wide char
		}
	}
}

// History reports the absolute line indexes of the oldest line kept and of
// the screen's first row, and whether a full-screen program is using the
// alternate screen (which has no history).
func (p *Pane) History() (first, screenTop int, alt bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term.FirstLine(), p.term.Pushed(), p.term.AltScreen()
}

// Search finds query in the pane's history and screen; see vt.Search.
func (p *Pane) Search(query string, from, dir int) (abs, col, width int, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term.Search(query, from, dir)
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

// Activity reports when the pane's program last produced output.
func (p *Pane) Activity() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastOut
}

// fgCacheFor is how long a foreground-process lookup is reused.
const fgCacheFor = 700 * time.Millisecond

// Foreground reports the program the pane's terminal is currently giving
// input to, and whether that is just the pane's shell (so: nothing is
// running). name is "" where the platform can't tell (Windows).
func (p *Pane) Foreground() (pid int, name string, isShell bool) {
	p.mu.Lock()
	dead := p.dead
	if !dead && time.Since(p.fgAt) > fgCacheFor {
		p.mu.Unlock()
		pid, name = p.proc.Foreground() // no lock: this can read /proc or run ps
		p.mu.Lock()
		p.fgPID, p.fgName, p.fgAt = pid, name, time.Now()
	}
	pid, name = p.fgPID, p.fgName
	p.mu.Unlock()

	if dead {
		return 0, "", false
	}
	// By pid, not by name: a nested shell (`sh` inside sh, a subshell)
	// is a running program, not this pane's idle prompt.
	return pid, name, pid != 0 && pid == p.proc.Pid()
}

// Capture returns the pane's text: the last n lines of the visible
// screen, or of the scrollback plus screen when history is true. Lines
// keep their left padding and lose trailing blanks, and blank lines below
// the last output are left out.
func (p *Pane) Capture(n int, history bool) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, rows := p.term.Size()
	last := p.term.Pushed() + rows // one past the last screen line
	first := p.term.Pushed()
	if history {
		first = p.term.FirstLine()
	}
	// The blank rows below the last output aren't content: an agent asking
	// for the last 3 lines wants text, not the bottom of an empty screen.
	for last > first && strings.TrimSpace(lineText(p.term.LineAt(last-1))) == "" {
		last--
	}
	if n > 0 && last-n > first {
		first = last - n
	}
	out := make([]string, 0, last-first)
	for abs := first; abs < last; abs++ {
		out = append(out, lineText(p.term.LineAt(abs)))
	}
	return out
}

func lineText(cells []vt.Cell) string {
	var b strings.Builder
	for _, c := range cells {
		if c.Wide != vt.WideTail {
			b.WriteString(c.String())
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Cwd returns the working directory of the pane's shell, or "" where the
// platform doesn't expose it.
func (p *Pane) Cwd() string { return p.proc.Cwd() }
