package ui

import (
	"io"
	"strconv"

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
	// Mouse asks the terminal to report clicks, drags and the wheel.
	// slat needs the reports whatever the pane's program wants, because a
	// click picks the pane the event then belongs to.
	Mouse bool
}

// mouseModes is what slat turns on to receive reports: button events with
// drag (1002) in SGR encoding (1006), which is the only one that works
// past column 223.

// Screen paints frames onto a real terminal. It remembers what the terminal
// currently shows and sends only the cells that changed, so redraws are
// cheap enough to do after every PTY read.
type Screen struct {
	w      io.Writer
	prev   *Frame // what the terminal shows; nil = unknown
	spare  *Frame // recycled for the next frame
	cursor Cursor
	modes  Modes
	out    []byte // reused across frames so Render allocates 0 bytes in steady state
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
	b := s.out[:0]
	full := s.prev == nil || s.prev.W != frame.W || s.prev.H != frame.H
	if full {
		b = append(b, "\x1b[0m\x1b[?25l\x1b[H\x1b[2J"...)
		// Unknown terminal state: set every mode explicitly.
		b = appendModeSeq(b, 1, modes.AppCursor)
		b = appendModeSeq(b, 2004, modes.BracketedPaste)
		b = appendMouseSeq(b, modes.Mouse)
		b = appendCursorStyle(b, cur.Style)
	} else {
		if modes.AppCursor != s.modes.AppCursor {
			b = appendModeSeq(b, 1, modes.AppCursor)
		}
		if modes.BracketedPaste != s.modes.BracketedPaste {
			b = appendModeSeq(b, 2004, modes.BracketedPaste)
		}
		if modes.Mouse != s.modes.Mouse {
			b = appendMouseSeq(b, modes.Mouse)
		}
		if cur.Style != s.cursor.Style {
			b = appendCursorStyle(b, cur.Style)
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
			if !full && c == old[x] && (cx != x || cy != y || !changedWithin(line, old, x, 4)) {
				x += w
				continue
			}
			if !painted {
				painted = true
				if !full {
					b = append(b, "\x1b[?25l"...)
				}
			}
			if cx != x || cy != y {
				b = appendCSI2(b, y+1, x+1, 'H')
			}
			if c.Style != pen {
				b = c.Style.AppendSGR(b)
				pen = c.Style
			}
			b = c.AppendTo(b)
			x += w
			cx, cy = x, y
		}
	}
	if pen != (vt.Style{}) {
		b = append(b, "\x1b[0m"...)
	}
	if len(b) == 0 && cur == s.cursor {
		s.spare = frame // nothing changed; reuse it
		return nil
	}
	if cur.Visible {
		b = appendCSI2(b, cur.Y+1, cur.X+1, 'H')
		b = append(b, "\x1b[?25h"...)
	} else {
		b = append(b, "\x1b[?25l"...)
	}

	s.out = b
	s.spare, s.prev = s.prev, frame
	s.cursor = cur
	s.modes = modes
	_, err := s.w.Write(b)
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

func appendModeSeq(dst []byte, mode int, on bool) []byte {
	dst = append(dst, "\x1b[?"...)
	dst = strconv.AppendInt(dst, int64(mode), 10)
	if on {
		return append(dst, 'h')
	}
	return append(dst, 'l')
}

func appendMouseSeq(dst []byte, on bool) []byte {
	if on {
		return append(dst, "\x1b[?1002h\x1b[?1006h"...)
	}
	return append(dst, "\x1b[?1002l\x1b[?1006l"...)
}

func appendCursorStyle(dst []byte, style int) []byte {
	dst = append(dst, "\x1b["...)
	dst = strconv.AppendInt(dst, int64(style), 10)
	return append(dst, ' ', 'q')
}

func appendCSI2(dst []byte, a, c int, suffix byte) []byte {
	dst = append(dst, "\x1b["...)
	dst = strconv.AppendInt(dst, int64(a), 10)
	dst = append(dst, ';')
	dst = strconv.AppendInt(dst, int64(c), 10)
	return append(dst, suffix)
}
