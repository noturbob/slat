package app

import (
	"fmt"

	"github.com/noturbob/slat/internal/pane"
)

// A mouse report from the real terminal. slat asks for SGR encoding
// (DECSET 1006), so that is the only form it has to read: coordinates are
// decimal and unlimited, where the original encoding capped them at 223.
type mouseEvent struct {
	button  int  // 0 left, 1 middle, 2 right, 64/65 wheel up/down
	x, y    int  // 1-based, in screen cells
	press   bool // false for a release (SGR "m")
	motion  bool // reported while a button is held
	shift   bool
	alt     bool
	control bool
}

func (e mouseEvent) wheel() int {
	switch e.button {
	case 64:
		return -1 // up: back into history
	case 65:
		return 1
	}
	return 0
}

// parseMouse reads an SGR mouse report starting at buf[i]:
// ESC [ < button ; x ; y (M press | m release). It returns how many bytes
// the report occupied, or ok=false when buf[i:] is not one.
func parseMouse(buf []byte, i int) (ev mouseEvent, n int, ok bool) {
	if i+3 >= len(buf) || buf[i] != 0x1b || buf[i+1] != '[' || buf[i+2] != '<' {
		return ev, 0, false
	}
	var nums [3]int
	field, j := 0, i+3
	for ; j < len(buf); j++ {
		c := buf[j]
		switch {
		case c >= '0' && c <= '9':
			nums[field] = nums[field]*10 + int(c-'0')
		case c == ';':
			if field == 2 {
				return ev, 0, false
			}
			field++
		case c == 'M' || c == 'm':
			if field != 2 {
				return ev, 0, false
			}
			b := nums[0]
			ev = mouseEvent{
				// Strip shift/alt/control (0x04/0x08/0x10) and the motion bit
				// (0x20); the wheel bit (0x40) is part of the button number.
				button:  b &^ 0x3c,
				x:       nums[1],
				y:       nums[2],
				press:   c == 'M',
				motion:  b&0x20 != 0,
				shift:   b&0x04 != 0,
				alt:     b&0x08 != 0,
				control: b&0x10 != 0,
			}
			return ev, j - i + 1, true
		default:
			return ev, 0, false
		}
	}
	return ev, 0, false // a report split across reads; drop it
}

// paneAt returns the pane of the active tab drawn over screen cell (x, y),
// both 1-based, or nil for a border or the status bar.
func (a *App) paneAt(x, y int) *pane.Pane {
	for _, p := range a.manager.ActivePanes() {
		row, col, rows, cols := p.Rect()
		if y-1 >= row && y-1 < row+rows && x-1 >= col && x-1 < col+cols {
			return p
		}
	}
	return nil
}

// mouseKey handles one mouse report. Called with the lock held; returns the
// index of its last byte so FeedInput's loop can step past it.
func (a *App) mouseKey(buf []byte, i int) int {
	ev, n, ok := parseMouse(buf, i)
	if !ok {
		return i
	}
	target := a.paneAt(ev.x, ev.y)

	// In scroll mode the wheel drives the view, whatever the pane runs.
	if a.scroll != nil {
		if d := ev.wheel(); d != 0 {
			a.scrollBy(d * 3)
		}
		return i + n - 1
	}

	// A click moves focus first, so the program that gets the event is the
	// one the user pointed at.
	if target != nil && ev.press && ev.button <= 2 && !ev.motion {
		a.manager.Focus(target)
	}

	if target != nil && a.forwardMouse(target, ev) {
		return i + n - 1
	}

	// Nothing wants it: the wheel scrolls back, like a terminal's own.
	if d := ev.wheel(); d < 0 && target != nil {
		if a.enterScroll() {
			a.scrollBy(-3)
		}
	}
	return i + n - 1
}

// forwardMouse sends the event to a pane's program in the encoding it asked
// for, with coordinates relative to the pane. It reports whether the program
// wanted the event at all.
func (a *App) forwardMouse(p *pane.Pane, ev mouseEvent) bool {
	mode, sgr := p.MouseMode()
	if mode == 0 {
		return false
	}
	// 1000 is click-only: motion is reported by 1002 (while held) and 1003.
	if ev.motion && mode == 1000 {
		return false
	}
	row, col, _, _ := p.Rect()
	if out := encodeMouse(ev, sgr, ev.x-col, ev.y-row); out != nil {
		p.Write(out)
	}
	return true
}

// encodeMouse renders an event at pane-local (x, y), 1-based, the way the
// program asked for it. It returns nil when the event cannot be expressed,
// which only the original encoding can manage: it spends one byte per
// coordinate, so nothing past column 223 fits.
func encodeMouse(ev mouseEvent, sgr bool, x, y int) []byte {
	b := ev.button
	if ev.motion {
		b |= 0x20
	}
	if ev.shift {
		b |= 0x04
	}
	if ev.alt {
		b |= 0x08
	}
	if ev.control {
		b |= 0x10
	}
	if sgr {
		final := byte('m')
		if ev.press {
			final = 'M'
		}
		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", b, x, y, final))
	}
	// The original encoding: one byte each, offset by 32, and a release is
	// reported as button 3 rather than with a separate final character.
	if !ev.press {
		b = 3
	}
	if x < 1 || y < 1 || x > 223 || y > 223 {
		return nil
	}
	return []byte{0x1b, '[', 'M', byte(32 + b), byte(32 + x), byte(32 + y)}
}
