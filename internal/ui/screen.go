package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/noturbob/slat/internal/vt"
)

// Frame is one full screen of cells, composed before it's rendered.
type Frame struct {
	W, H  int
	Lines [][]vt.Cell
	cells []vt.Cell // backing store of Lines
}

// NewFrame returns a blank w x h frame.
func NewFrame(w, h int) *Frame {
	f := &Frame{W: w, H: h, Lines: make([][]vt.Cell, h), cells: make([]vt.Cell, w*h)}
	for y := range f.Lines {
		f.Lines[y] = f.cells[y*w : (y+1)*w]
	}
	return f
}

// Set puts c at (x, y), clipped to the frame. Overwriting either half of
// a wide character blanks the other half, so none is left half-drawn.
func (f *Frame) Set(x, y int, c vt.Cell) {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return
	}
	line := f.Lines[y]
	switch line[x].Wide {
	case vt.WideHead:
		if x+1 < f.W {
			line[x+1] = vt.Cell{Style: line[x+1].Style}
		}
	case vt.WideTail:
		if x > 0 {
			line[x-1] = vt.Cell{Style: line[x-1].Style}
		}
	}
	if c.Wide == vt.WideHead && x+1 >= f.W {
		c = vt.Cell{Style: c.Style} // no room for the second half
	}
	line[x] = c
}

// Cursor is where the real terminal's cursor should rest after a frame.
type Cursor struct {
	X, Y    int
	Visible bool
	Style   int // DECSCUSR parameter, 0 = terminal default
}

// Modes are input modes to mirror onto the real terminal, so keys and
// pastes reach the active pane's program in the encoding it asked for.
type Modes struct {
	AppCursor      bool
	BracketedPaste bool
}

// Screen paints frames onto a real terminal. It remembers what the terminal
// currently shows and sends only the cells that changed, so redraws are
// cheap enough to do after every PTY read.
type Screen struct {
	w      io.Writer
	prev   *Frame // what the terminal shows; nil = unknown
	spare  *Frame // recycled for the next frame
	cursor Cursor
	modes  Modes
	out    strings.Builder
}

// NewScreen returns a Screen that writes to w.
func NewScreen(w io.Writer) *Screen { return &Screen{w: w} }

// WriteRaw sends bytes straight to the terminal, outside the frame
// diffing — for sequences the terminal itself must see, like the OSC 52
// that puts text on the system clipboard.
func (s *Screen) WriteRaw(p []byte) error {
	_, err := s.w.Write(p)
	return err
}

// Invalidate forgets what the terminal shows, forcing the next Render to
// repaint everything, e.g. after a new client attaches.
func (s *Screen) Invalidate() { s.prev = nil }

// NextFrame returns a blank w x h frame to compose into, reusing the
// memory of the frame before last.
func (s *Screen) NextFrame(w, h int) *Frame {
	f := s.spare
	s.spare = nil
	if f == nil || f.W != w || f.H != h {
		return NewFrame(w, h)
	}
	clear(f.cells)
	return f
}

// Render brings the terminal up to date with frame. The Screen keeps
// frame, so the caller must not modify it afterwards.
func (s *Screen) Render(frame *Frame, cur Cursor, modes Modes) error {
	b := &s.out
	b.Reset()
	full := s.prev == nil || s.prev.W != frame.W || s.prev.H != frame.H
	if full {
		b.WriteString("\x1b[0m\x1b[?25l\x1b[H\x1b[2J")
		// Unknown terminal state: set every mode explicitly.
		b.WriteString(modeSeq(1, modes.AppCursor) + modeSeq(2004, modes.BracketedPaste))
		fmt.Fprintf(b, "\x1b[%d q", cur.Style)
	} else {
		if modes.AppCursor != s.modes.AppCursor {
			b.WriteString(modeSeq(1, modes.AppCursor))
		}
		if modes.BracketedPaste != s.modes.BracketedPaste {
			b.WriteString(modeSeq(2004, modes.BracketedPaste))
		}
		if cur.Style != s.cursor.Style {
			fmt.Fprintf(b, "\x1b[%d q", cur.Style)
		}
	}

	painted := false
	var pen vt.Style
	cx, cy := -1, -1 // where the terminal cursor is after our writes
	for y, line := range frame.Lines {
		var old []vt.Cell
		if !full {
			old = s.prev.Lines[y]
		}
		for x := 0; x < len(line); {
			c := line[x]
			if c.Wide == vt.WideTail {
				x++ // drawn by the wide character before it
				continue
			}
			w := 1
			if c.Wide == vt.WideHead {
				w = 2
			}
			// Skip unchanged cells, unless the cursor is already here and
			// something changes within a few cells: rewriting a short gap is
			// cheaper than a cursor jump over it.
			if !full && c == old[x] && !(cx == x && cy == y && changedWithin(line, old, x, 4)) {
				x += w
				continue
			}
			if !painted {
				painted = true
				if !full {
					b.WriteString("\x1b[?25l")
				}
			}
			if cx != x || cy != y {
				fmt.Fprintf(b, "\x1b[%d;%dH", y+1, x+1)
			}
			if c.Style != pen {
				b.WriteString(c.Style.SGR())
				pen = c.Style
			}
			b.WriteString(c.String())
			x += w
			cx, cy = x, y
		}
	}
	if pen != (vt.Style{}) {
		b.WriteString("\x1b[0m")
	}
	if b.Len() == 0 && cur == s.cursor {
		s.spare = frame // nothing changed; reuse it
		return nil
	}
	if cur.Visible {
		fmt.Fprintf(b, "\x1b[%d;%dH\x1b[?25h", cur.Y+1, cur.X+1)
	} else {
		b.WriteString("\x1b[?25l")
	}

	s.spare, s.prev = s.prev, frame
	s.cursor = cur
	s.modes = modes
	_, err := io.WriteString(s.w, b.String())
	return err
}

// changedWithin reports whether any of the n cells from x differs.
func changedWithin(line, old []vt.Cell, x, n int) bool {
	for i := x; i < x+n && i < len(line); i++ {
		if line[i] != old[i] {
			return true
		}
	}
	return false
}

func modeSeq(mode int, on bool) string {
	if on {
		return fmt.Sprintf("\x1b[?%dh", mode)
	}
	return fmt.Sprintf("\x1b[?%dl", mode)
}
