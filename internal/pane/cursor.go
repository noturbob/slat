package pane

import "unicode/utf8"

// cursorState tracks a minimal terminal cursor position for the bytes
// written into this pane's PTY output stream.
//
// The host renders every pane by streaming its raw PTY bytes straight onto
// the real terminal (there is no per-pane screen buffer). That only looks
// right if the real cursor is already sitting exactly where this pane's
// shell believes its own cursor is before each write. Without tracking
// that position ourselves, the assumption breaks the instant anything else
// touches the shared screen in between writes to this pane -- a border
// redraw, the status bar, or output from another pane -- and the pane's
// next write lands wherever the cursor happened to be left, not where it
// belongs. That desync is what produced symptoms like prompts rendering on
// the wrong line/pane after switching focus or splitting.
//
// This is intentionally not a full terminal emulator: no screen buffer, so
// reattaching a client can't replay pane history. It only tracks enough
// state -- cursor row/col, deferred autowrap, one saved position -- to keep
// the physical cursor in sync with what the pane's shell thinks it is.
type cursorState struct {
	row, col           int // 0-based, relative to the pane's own top-left cell
	wrapPend           bool
	savedRow, savedCol int

	esc     bool
	csi     bool
	strSeq  bool
	skip    int // remaining bytes to swallow with no effect (charset designators etc.)
	params  []int
	cur     int
	haveCur bool
}

// clamp keeps the tracked position inside rows x cols, e.g. after a resize.
func (c *cursorState) clamp(rows, cols int) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	if c.row > rows-1 {
		c.row = rows - 1
	}
	if c.row < 0 {
		c.row = 0
	}
	if c.col > cols-1 {
		c.col = cols - 1
	}
	if c.col < 0 {
		c.col = 0
	}
	c.wrapPend = false
}

func (c *cursorState) lineFeed(rows int) {
	if c.row < rows-1 {
		c.row++
	}
	// Otherwise the cursor is pinned at the pane's bottom row; the real
	// terminal's own scroll region (set by the host to match this pane's
	// rows before writing) performs the visual scroll for us.
}

func (c *cursorState) printRune(rows, cols int) {
	if c.wrapPend {
		c.col = 0
		c.lineFeed(rows)
		c.wrapPend = false
	}
	if c.col >= cols-1 {
		c.col = cols - 1
		c.wrapPend = true
	} else {
		c.col++
	}
}

// feed interprets data as bytes written to a rows x cols terminal starting
// at the pane's current tracked position, updating that position in place.
func (c *cursorState) feed(rows, cols int, data []byte) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}

	i := 0
	for i < len(data) {
		b := data[i]

		if c.skip > 0 {
			c.skip--
			i++
			continue
		}

		if c.strSeq {
			// Inside an OSC/DCS/PM/APC string: skip to BEL or ST (ESC \).
			if b == 0x07 {
				c.strSeq = false
			} else if b == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
				c.strSeq = false
				i++
			}
			i++
			continue
		}

		if c.csi {
			switch {
			case b >= '0' && b <= '9':
				if !c.haveCur {
					c.haveCur = true
					c.cur = 0
				}
				c.cur = c.cur*10 + int(b-'0')
			case b == ';':
				c.params = append(c.params, c.cur)
				c.cur = 0
				c.haveCur = false
			case b >= 0x40 && b <= 0x7e:
				if c.haveCur || len(c.params) == 0 {
					c.params = append(c.params, c.cur)
				}
				c.applyCSI(b, c.params, rows, cols)
				c.csi = false
				c.params = nil
				c.cur = 0
				c.haveCur = false
			default:
				// Intermediate bytes (0x20-0x2F) and private markers
				// (?, >, =): no effect on the subset we track.
			}
			i++
			continue
		}

		if c.esc {
			c.esc = false
			switch b {
			case '[':
				c.csi = true
				c.params = nil
				c.cur = 0
				c.haveCur = false
			case ']', 'P', '^', '_', 'X':
				c.strSeq = true
			case '7':
				c.savedRow, c.savedCol = c.row, c.col
			case '8':
				c.row, c.col = c.savedRow, c.savedCol
				c.wrapPend = false
			case 'D': // IND
				c.lineFeed(rows)
			case 'M': // RI
				if c.row > 0 {
					c.row--
				}
				c.wrapPend = false
			case 'E': // NEL
				c.lineFeed(rows)
				c.col = 0
				c.wrapPend = false
			case '(', ')', '*', '+', '#', '%':
				c.skip = 1 // charset/line-size designators: one more byte
			default:
				// Unknown single-char escape: no cursor effect we track.
			}
			i++
			continue
		}

		switch {
		case b == 0x1b:
			c.esc = true
		case b == '\r':
			c.col = 0
			c.wrapPend = false
		case b == '\n' || b == '\v' || b == '\f':
			c.lineFeed(rows)
			c.wrapPend = false
		case b == '\b':
			if c.col > 0 {
				c.col--
			}
			c.wrapPend = false
		case b == '\t':
			next := ((c.col / 8) + 1) * 8
			if next > cols-1 {
				next = cols - 1
			}
			c.col = next
			c.wrapPend = false
		case b < 0x20 || b == 0x7f:
			// Other control bytes: no cursor effect.
		default:
			_, size := utf8.DecodeRune(data[i:])
			if size <= 0 {
				size = 1
			}
			c.printRune(rows, cols)
			i += size
			continue
		}
		i++
	}
}

func (c *cursorState) applyCSI(final byte, params []int, rows, cols int) {
	get := func(i, def int) int {
		if i < len(params) && params[i] > 0 {
			return params[i]
		}
		return def
	}
	switch final {
	case 'A': // CUU
		c.row -= get(0, 1)
		c.wrapPend = false
	case 'B': // CUD
		c.row += get(0, 1)
		c.wrapPend = false
	case 'C': // CUF
		c.col += get(0, 1)
		c.wrapPend = false
	case 'D': // CUB
		c.col -= get(0, 1)
		c.wrapPend = false
	case 'E': // CNL
		c.row += get(0, 1)
		c.col = 0
		c.wrapPend = false
	case 'F': // CPL
		c.row -= get(0, 1)
		c.col = 0
		c.wrapPend = false
	case 'G', '`': // CHA / HPA
		c.col = get(0, 1) - 1
		c.wrapPend = false
	case 'd': // VPA
		c.row = get(0, 1) - 1
		c.wrapPend = false
	case 'H', 'f': // CUP / HVP
		c.row = get(0, 1) - 1
		c.col = get(1, 1) - 1
		c.wrapPend = false
	case 's': // SCP (ANSI save cursor)
		c.savedRow, c.savedCol = c.row, c.col
	case 'u': // RCP (ANSI restore cursor)
		c.row, c.col = c.savedRow, c.savedCol
		c.wrapPend = false
	default:
		// Erase (J/K), SGR (m), mode set (h/l), scroll region (r), etc:
		// no cursor position effect we need to track.
		return
	}
	if c.row < 0 {
		c.row = 0
	}
	if c.row > rows-1 {
		c.row = rows - 1
	}
	if c.col < 0 {
		c.col = 0
	}
	if c.col > cols-1 {
		c.col = cols - 1
	}
}
