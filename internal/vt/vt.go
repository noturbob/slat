// Package vt is the terminal emulator behind every pane: it interprets a
// program's output (the xterm subset that shells, editors, pagers and TUIs
// use) into a grid of cells that slat can repaint at any time.
//
// It keeps no scrollback, and scrolling rotates rows rather than copying
// cells, so a pane can absorb output as fast as a program produces it.
package vt

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// Attr is a set of text attributes.
type Attr uint16

const (
	Bold Attr = 1 << iota
	Faint
	Italic
	Underline
	Blink
	Reverse
	Invisible
	Strike
)

// Color is a cell color: 0 is the terminal's default, otherwise the top
// byte says whether the rest is a palette index or 24-bit RGB.
type Color uint32

const (
	DefaultColor Color = 0
	indexedFlag  Color = 1 << 24
	rgbFlag      Color = 2 << 24
)

// Indexed returns palette color n (0-255).
func Indexed(n uint8) Color { return indexedFlag | Color(n) }

// RGB returns a 24-bit color.
func RGB(r, g, b uint8) Color { return rgbFlag | Color(r)<<16 | Color(g)<<8 | Color(b) }

// Style is how a cell is drawn. The zero Style is the terminal default.
type Style struct {
	Fg, Bg Color
	Attrs  Attr
}

// SGR returns the escape sequence that selects s from a reset state.
func (s Style) SGR() string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	for i, a := range []Attr{Bold, Faint, Italic, Underline, Blink, Reverse, Invisible, Strike} {
		if s.Attrs&a != 0 {
			b.WriteString([]string{";1", ";2", ";3", ";4", ";5", ";7", ";8", ";9"}[i])
		}
	}
	writeColor(&b, s.Fg, 30, 90, 38)
	writeColor(&b, s.Bg, 40, 100, 48)
	b.WriteByte('m')
	return b.String()
}

func writeColor(b *strings.Builder, c Color, base, bright, ext int) {
	switch {
	case c == DefaultColor:
	case c&rgbFlag != 0:
		fmt.Fprintf(b, ";%d;2;%d;%d;%d", ext, c>>16&0xff, c>>8&0xff, c&0xff)
	case c&0xff < 8:
		fmt.Fprintf(b, ";%d", base+int(c&0xff))
	case c&0xff < 16:
		fmt.Fprintf(b, ";%d", bright+int(c&0xff)-8)
	default:
		fmt.Fprintf(b, ";%d;5;%d", ext, c&0xff)
	}
}

// Wide-character markers for Cell.Wide.
const (
	Narrow   = 0
	WideHead = 1 // first column of a double-width character
	WideTail = 2 // second column; draws nothing
)

// Cell is one screen position. The zero Cell is a blank in default style.
type Cell struct {
	R     rune   // 0 means blank
	Comb  string // combining marks following R, usually empty
	Wide  uint8
	Style Style
}

// String returns what the cell displays.
func (c Cell) String() string {
	if c.R == 0 {
		return " "
	}
	if c.Comb == "" {
		return string(c.R)
	}
	return string(c.R) + c.Comb
}

// width is fixed at "ambiguous = narrow", what nearly every terminal does;
// runewidth's default would switch with the locale.
var width = &runewidth.Condition{}

// RuneWidth is the number of columns r occupies.
func RuneWidth(r rune) int { return width.RuneWidth(r) }

// StringWidth is the number of columns s occupies.
func StringWidth(s string) int { return width.StringWidth(s) }

// Truncate cuts s to at most w columns, ending in tail if it was cut.
func Truncate(s string, w int, tail string) string { return width.Truncate(s, w, tail) }

type cursor struct {
	x, y     int
	pen      Style
	wrapNext bool // at the right margin: the next print wraps first
	origin   bool // DECOM: row addressing is relative to the scroll region
	g        [2]bool
	gl       int // active charset (0 = G0, 1 = G1); g[i] = DEC line drawing
}

type parseState int

const (
	ground parseState = iota
	escape
	escInter
	csiEntry
	oscString
	ignoreString // DCS / SOS / PM / APC: consumed, not interpreted
)

// Terminal is a virtual terminal. It is not safe for concurrent use.
type Terminal struct {
	cols, rows int
	main, alt  [][]Cell
	lines      [][]Cell // main or alt
	cur        cursor
	saved      [2]cursor // DECSC per screen (main, alt)
	top, bot   int       // scroll region, inclusive
	tabs       []bool
	last       rune // last printed rune, for REP

	autowrap, insert, altScreen bool
	appCursor, bracketedPaste   bool
	cursorHidden                bool
	cursorStyle                 int

	state   parseState
	utf     []byte
	params  []byte // raw CSI parameter bytes
	inter   []byte // intermediate bytes
	private byte   // CSI private marker: '?', '>', '<', '=' or 0
	strEsc  bool   // saw ESC inside a string; waiting for '\'

	replies []byte

	// Scrollback ring; see history.go.
	hist                        [][]Cell
	histStart, histLen, histMax int
	pushed                      int
}

// New returns a cols x rows terminal.
func New(cols, rows int) *Terminal {
	t := &Terminal{}
	t.cols, t.rows = max(cols, 1), max(rows, 1)
	t.main, t.alt = newLines(t.cols, t.rows), newLines(t.cols, t.rows)
	t.reset()
	return t
}

func newLines(cols, rows int) [][]Cell {
	lines := make([][]Cell, rows)
	for i := range lines {
		lines[i] = make([]Cell, cols)
	}
	return lines
}

func (t *Terminal) reset() {
	t.lines = t.main
	t.altScreen = false
	t.clearLines(t.main, Cell{})
	t.clearLines(t.alt, Cell{})
	t.cur = cursor{}
	t.saved = [2]cursor{}
	t.top, t.bot = 0, t.rows-1
	t.resetTabs()
	t.autowrap, t.insert = true, false
	t.appCursor, t.bracketedPaste, t.cursorHidden = false, false, false
	t.cursorStyle = 0
	t.state = ground
}

func (t *Terminal) resetTabs() {
	t.tabs = make([]bool, t.cols)
	for i := 8; i < t.cols; i += 8 {
		t.tabs[i] = true
	}
}

func (t *Terminal) clearLines(lines [][]Cell, blank Cell) {
	for _, l := range lines {
		for i := range l {
			l[i] = blank
		}
	}
}

// ─── Accessors ──────────────────────────────────────────────────────────────

// Size returns the terminal's dimensions.
func (t *Terminal) Size() (cols, rows int) { return t.cols, t.rows }

// Cell returns the cell at column x, row y.
func (t *Terminal) Cell(x, y int) Cell { return t.lines[y][x] }

// Line returns row y. The slice is only valid until the next Write/Resize.
func (t *Terminal) Line(y int) []Cell { return t.lines[y] }

// Cursor returns the cursor position.
func (t *Terminal) Cursor() (x, y int) { return t.cur.x, t.cur.y }

// CursorVisible reports whether the program wants the cursor shown.
func (t *Terminal) CursorVisible() bool { return !t.cursorHidden }

// CursorStyle is the DECSCUSR parameter the program set (0 = default).
func (t *Terminal) CursorStyle() int { return t.cursorStyle }

// AppCursor reports DECCKM: arrow keys should be sent as ESC O x.
func (t *Terminal) AppCursor() bool { return t.appCursor }

// BracketedPaste reports whether pastes should be bracketed.
func (t *Terminal) BracketedPaste() bool { return t.bracketedPaste }

// AltScreen reports whether the alternate screen is active.
func (t *Terminal) AltScreen() bool { return t.altScreen }

// Replies returns and clears the terminal's answers to queries (cursor
// position reports, device attributes), which must be sent back to the
// program as input.
func (t *Terminal) Replies() []byte {
	r := t.replies
	t.replies = nil
	return r
}

// String returns the screen as text, one line per row, trailing blanks
// trimmed.
func (t *Terminal) String() string {
	var b strings.Builder
	for y, l := range t.lines {
		var line strings.Builder
		for _, c := range l {
			if c.Wide != WideTail {
				line.WriteString(c.String())
			}
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		if y < len(t.lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ─── Resize ─────────────────────────────────────────────────────────────────

// Resize changes the terminal's size, keeping the cursor's line on screen.
// Lines are truncated or padded, not reflowed.
func (t *Terminal) Resize(cols, rows int) {
	cols, rows = max(cols, 1), max(rows, 1)
	if cols == t.cols && rows == t.rows {
		return
	}
	// When shrinking, drop lines from the top rather than losing the
	// cursor's line (where the shell's prompt is).
	shift := max(t.cur.y-(rows-1), 0)
	resize := func(old [][]Cell, shift int) [][]Cell {
		lines := newLines(cols, rows)
		for y := range lines {
			if y+shift < len(old) {
				copy(lines[y], old[y+shift])
				if l := lines[y]; l[cols-1].Wide == WideHead {
					l[cols-1] = Cell{Style: l[cols-1].Style} // half a wide char
				}
			}
		}
		return lines
	}
	mainShift, altShift := shift, shift
	if t.altScreen {
		mainShift = max(t.saved[0].y-(rows-1), 0)
	} else {
		altShift = 0
	}
	for _, l := range t.main[:mainShift] {
		t.pushHistory(l) // rows pushed off the top by a shrink
	}
	t.main, t.alt = resize(t.main, mainShift), resize(t.alt, altShift)
	t.lines = t.main
	if t.altScreen {
		t.lines = t.alt
	}
	t.cols, t.rows = cols, rows
	t.top, t.bot = 0, rows-1
	t.resetTabs()
	t.cur.y -= shift
	t.saved[t.screenIdx()].y -= shift
	t.clampCursor(&t.cur)
	for i := range t.saved {
		t.clampCursor(&t.saved[i])
	}
}

func (t *Terminal) clampCursor(c *cursor) {
	c.x = min(max(c.x, 0), t.cols-1)
	c.y = min(max(c.y, 0), t.rows-1)
	c.wrapNext = false
}

// ─── Parser ─────────────────────────────────────────────────────────────────

// Write feeds program output to the terminal.
func (t *Terminal) Write(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		b := p[i]
		switch t.state {
		case ground:
			if b >= 0x20 && b < 0x7f && len(t.utf) == 0 {
				// Fast path for runs of printable ASCII.
				t.print(rune(b))
				continue
			}
			t.ground(b)
		case escape:
			t.escape(b)
		case escInter:
			t.escInter(b)
		case csiEntry:
			t.csiByte(b)
		case oscString, ignoreString:
			t.stringByte(b)
		}
	}
	return len(p), nil
}

func (t *Terminal) ground(b byte) {
	if len(t.utf) > 0 {
		if b&0xc0 == 0x80 { // continuation byte
			t.utf = append(t.utf, b)
			if utf8.FullRune(t.utf) {
				r, _ := utf8.DecodeRune(t.utf)
				t.utf = t.utf[:0]
				t.print(r)
			}
			return
		}
		t.utf = t.utf[:0]
		t.print(utf8.RuneError) // truncated sequence; b is handled below
	}
	switch {
	case b >= 0xc2 && b <= 0xf4:
		t.utf = append(t.utf, b)
	case b >= 0x80:
		t.print(utf8.RuneError) // stray continuation or invalid byte
	case b == 0x1b:
		t.state = escape
		t.inter = t.inter[:0]
	case b < 0x20 || b == 0x7f:
		t.control(b)
	default:
		t.print(rune(b))
	}
}

func (t *Terminal) control(b byte) {
	switch b {
	case '\b':
		if t.cur.x > 0 {
			t.cur.x--
		}
		t.cur.wrapNext = false
	case '\t':
		t.tab(1)
	case '\n', '\v', '\f':
		t.index()
	case '\r':
		t.cur.x = 0
		t.cur.wrapNext = false
	case 0x0e: // SO
		t.cur.gl = 1
	case 0x0f: // SI
		t.cur.gl = 0
	}
}

func (t *Terminal) escape(b byte) {
	t.state = ground
	switch b {
	case '[':
		t.state = csiEntry
		t.params, t.inter, t.private = t.params[:0], t.inter[:0], 0
	case ']':
		t.state = oscString
		t.params = t.params[:0]
	case 'P', 'X', '^', '_':
		t.state = ignoreString
	case '7':
		t.saveCursor()
	case '8':
		t.restoreCursor()
	case 'D':
		t.index()
	case 'E':
		t.cur.x = 0
		t.index()
	case 'M':
		t.reverseIndex()
	case 'H':
		t.tabs[t.cur.x] = true
	case 'c':
		t.reset()
	case 0x18, 0x1a: // CAN, SUB
	case 0x1b:
		t.state = escape
	default:
		if b >= 0x20 && b < 0x30 {
			t.inter = append(t.inter[:0], b)
			t.state = escInter
		} else if b < 0x20 {
			t.control(b)
			t.state = escape
		}
	}
}

// escInter handles ESC <intermediate> <final>, e.g. charset designation.
func (t *Terminal) escInter(b byte) {
	if b >= 0x20 && b < 0x30 {
		t.inter = append(t.inter, b)
		return
	}
	t.state = ground
	if len(t.inter) == 1 {
		switch t.inter[0] {
		case '(':
			t.cur.g[0] = b == '0'
		case ')':
			t.cur.g[1] = b == '0'
		case '#':
			if b == '8' { // DECALN: fill with E
				for _, l := range t.lines {
					for i := range l {
						l[i] = Cell{R: 'E'}
					}
				}
			}
		}
	}
}

func (t *Terminal) stringByte(b byte) {
	switch {
	case b == 0x07 && t.state == oscString: // BEL terminates OSC
		t.state = ground
	case t.strEsc:
		t.strEsc = false
		t.state = ground
		if b != '\\' {
			t.escape(b) // not ST: an ESC started a new sequence
		}
	case b == 0x1b:
		t.strEsc = true
	case b == 0x18 || b == 0x1a:
		t.state = ground
	}
	// OSC content (titles, colors, hyperlinks) isn't needed by slat.
}

func (t *Terminal) csiByte(b byte) {
	switch {
	case b >= 0x30 && b <= 0x3f:
		if len(t.params) == 0 && b >= '<' && t.private == 0 {
			t.private = b
		} else {
			t.params = append(t.params, b)
		}
	case b >= 0x20 && b <= 0x2f:
		t.inter = append(t.inter, b)
	case b >= 0x40 && b <= 0x7e:
		t.state = ground
		t.csi(b)
	case b == 0x1b:
		t.state = escape
		t.inter = t.inter[:0]
	case b == 0x18 || b == 0x1a:
		t.state = ground
	case b < 0x20:
		t.control(b) // C0 controls execute even inside a CSI
	}
}

// csiParams splits parameters into groups of colon-separated sub-params.
// Missing values are -1.
func (t *Terminal) csiParams() [][]int {
	if len(t.params) == 0 {
		return nil
	}
	var out [][]int
	for _, group := range strings.Split(string(t.params), ";") {
		var sub []int
		for _, s := range strings.Split(group, ":") {
			n, err := strconv.Atoi(s)
			if err != nil {
				n = -1
			}
			sub = append(sub, n)
		}
		out = append(out, sub)
	}
	return out
}

// ─── CSI ────────────────────────────────────────────────────────────────────

func (t *Terminal) csi(final byte) {
	ps := t.csiParams()
	// p returns parameter i, or def when it's missing or zero.
	p := func(i, def int) int {
		if i < len(ps) && ps[i][0] > 0 {
			return ps[i][0]
		}
		return def
	}
	inter := string(t.inter)

	switch t.private {
	case '?':
		switch final {
		case 'h', 'l':
			for _, g := range ps {
				t.setPrivateMode(g[0], final == 'h')
			}
		case 'J', 'K': // DECSED / DECSEL: erase (no protected cells here)
			t.private = 0
			t.csi(final)
		}
		return
	case '>':
		if final == 'c' && inter == "" { // secondary DA
			t.reply("\x1b[>1;10;0c")
		}
		return
	case 0:
	default:
		return // '<' / '=' sequences (keyboard protocols) are not supported
	}

	switch inter {
	case "":
	case " ":
		if final == 'q' { // DECSCUSR
			t.cursorStyle = min(p(0, 0), 6)
		}
		return
	case "!":
		if final == 'p' { // DECSTR soft reset
			t.cur.pen, t.cur.origin, t.insert, t.autowrap = Style{}, false, false, true
			t.top, t.bot, t.cursorHidden = 0, t.rows-1, false
		}
		return
	default:
		return
	}

	c := &t.cur
	switch final {
	case '@': // ICH
		t.insertCells(p(0, 1))
	case 'A':
		t.moveTo(c.x, max(c.y-p(0, 1), t.topLimit()))
	case 'B', 'e':
		t.moveTo(c.x, min(c.y+p(0, 1), t.botLimit()))
	case 'C', 'a':
		t.moveTo(c.x+p(0, 1), c.y)
	case 'D':
		t.moveTo(c.x-p(0, 1), c.y)
	case 'E':
		t.moveTo(0, min(c.y+p(0, 1), t.botLimit()))
	case 'F':
		t.moveTo(0, max(c.y-p(0, 1), t.topLimit()))
	case 'G', '`':
		t.moveTo(p(0, 1)-1, c.y)
	case 'H', 'f':
		t.moveAbs(p(1, 1)-1, p(0, 1)-1)
	case 'I':
		t.tab(p(0, 1))
	case 'J':
		t.eraseDisplay(p(0, 0))
	case 'K':
		t.eraseLine(p(0, 0))
	case 'L':
		t.insertLines(p(0, 1))
	case 'M':
		t.deleteLines(p(0, 1))
	case 'P':
		t.deleteCells(p(0, 1))
	case 'S':
		t.scrollUp(t.top, t.bot, p(0, 1))
	case 'T':
		if len(ps) <= 1 {
			t.scrollDown(t.top, t.bot, p(0, 1))
		}
	case 'X':
		n := min(p(0, 1), t.cols-c.x)
		t.fill(c.y, c.x, c.x+n)
		c.wrapNext = false
	case 'Z':
		t.tab(-p(0, 1))
	case 'b': // REP
		if t.last != 0 {
			for range min(p(0, 1), 65535) {
				t.print(t.last)
			}
		}
	case 'c':
		if p(0, 0) == 0 {
			t.reply("\x1b[?62;22c") // VT220 with ANSI color
		}
	case 'd':
		t.moveAbs(c.x, p(0, 1)-1)
	case 'g':
		switch p(0, 0) {
		case 0:
			t.tabs[c.x] = false
		case 3:
			clear(t.tabs)
		}
	case 'h', 'l':
		for _, g := range ps {
			if g[0] == 4 {
				t.insert = final == 'h'
			}
		}
	case 'm':
		t.sgr(ps)
	case 'n':
		switch p(0, 0) {
		case 5:
			t.reply("\x1b[0n")
		case 6:
			y := c.y
			if c.origin {
				y -= t.top
			}
			t.reply(fmt.Sprintf("\x1b[%d;%dR", y+1, c.x+1))
		}
	case 'r':
		top, bot := p(0, 1)-1, p(1, t.rows)-1
		if top < bot && bot < t.rows {
			t.top, t.bot = top, bot
			t.moveAbs(0, 0)
		}
	case 's':
		t.saveCursor()
	case 'u':
		t.restoreCursor()
	}
}

func (t *Terminal) reply(s string) { t.replies = append(t.replies, s...) }

func (t *Terminal) setPrivateMode(mode int, on bool) {
	switch mode {
	case 1:
		t.appCursor = on
	case 6:
		t.cur.origin = on
		t.moveAbs(0, 0)
	case 7:
		t.autowrap = on
		if !on {
			t.cur.wrapNext = false
		}
	case 25:
		t.cursorHidden = !on
	case 47, 1047:
		t.switchScreen(on, false)
	case 1048:
		if on {
			t.saveCursor()
		} else {
			t.restoreCursor()
		}
	case 1049:
		if on {
			t.saveCursor()
			t.switchScreen(true, true)
		} else {
			t.switchScreen(false, false)
			t.restoreCursor()
		}
	case 2004:
		t.bracketedPaste = on
	}
}

func (t *Terminal) switchScreen(alt, clearIt bool) {
	if alt == t.altScreen {
		return
	}
	t.altScreen = alt
	if alt {
		t.lines = t.alt
		if clearIt {
			t.clearLines(t.alt, Cell{})
		}
	} else {
		t.lines = t.main
	}
}

func (t *Terminal) screenIdx() int {
	if t.altScreen {
		return 1
	}
	return 0
}

func (t *Terminal) saveCursor() { t.saved[t.screenIdx()] = t.cur }

func (t *Terminal) restoreCursor() {
	t.cur = t.saved[t.screenIdx()]
	t.clampCursor(&t.cur)
}

// ─── SGR ────────────────────────────────────────────────────────────────────

func (t *Terminal) sgr(ps [][]int) {
	pen := &t.cur.pen
	if len(ps) == 0 {
		*pen = Style{}
		return
	}
	for i := 0; i < len(ps); i++ {
		g := ps[i]
		switch n := g[0]; {
		case n <= 0:
			*pen = Style{}
		case n == 1:
			pen.Attrs |= Bold
		case n == 2:
			pen.Attrs |= Faint
		case n == 3:
			pen.Attrs |= Italic
		case n == 4:
			if len(g) > 1 && g[1] == 0 {
				pen.Attrs &^= Underline // 4:0
			} else {
				pen.Attrs |= Underline
			}
		case n == 5 || n == 6:
			pen.Attrs |= Blink
		case n == 7:
			pen.Attrs |= Reverse
		case n == 8:
			pen.Attrs |= Invisible
		case n == 9:
			pen.Attrs |= Strike
		case n == 21:
			pen.Attrs |= Underline
		case n == 22:
			pen.Attrs &^= Bold | Faint
		case n == 23:
			pen.Attrs &^= Italic
		case n == 24:
			pen.Attrs &^= Underline
		case n == 25:
			pen.Attrs &^= Blink
		case n == 27:
			pen.Attrs &^= Reverse
		case n == 28:
			pen.Attrs &^= Invisible
		case n == 29:
			pen.Attrs &^= Strike
		case n >= 30 && n <= 37:
			pen.Fg = Indexed(uint8(n - 30))
		case n == 38, n == 48, n == 58:
			var c Color
			var ok bool
			if len(g) > 1 {
				c, ok = extColor(g[1:])
			} else {
				var used int
				c, used, ok = extColorSemicolon(ps[i+1:])
				i += used
			}
			if ok && n == 38 {
				pen.Fg = c
			} else if ok && n == 48 {
				pen.Bg = c
			} // 58 (underline color) is parsed only to skip its arguments
		case n == 39:
			pen.Fg = DefaultColor
		case n >= 40 && n <= 47:
			pen.Bg = Indexed(uint8(n - 40))
		case n == 49:
			pen.Bg = DefaultColor
		case n >= 90 && n <= 97:
			pen.Fg = Indexed(uint8(n - 90 + 8))
		case n >= 100 && n <= 107:
			pen.Bg = Indexed(uint8(n - 100 + 8))
		}
	}
}

// extColor parses colon form: 5:n, 2:r:g:b or 2:colorspace:r:g:b.
func extColor(sub []int) (Color, bool) {
	switch {
	case len(sub) >= 2 && sub[0] == 5:
		return Indexed(uint8(max(sub[1], 0))), true
	case len(sub) >= 5 && sub[0] == 2:
		return RGB(byte8(sub[2]), byte8(sub[3]), byte8(sub[4])), true
	case len(sub) >= 4 && sub[0] == 2:
		return RGB(byte8(sub[1]), byte8(sub[2]), byte8(sub[3])), true
	}
	return 0, false
}

// extColorSemicolon parses 38;5;n and 38;2;r;g;b, returning how many
// parameter groups it consumed.
func extColorSemicolon(rest [][]int) (Color, int, bool) {
	if len(rest) == 0 {
		return 0, 0, false
	}
	switch rest[0][0] {
	case 5:
		if len(rest) >= 2 {
			return Indexed(uint8(max(rest[1][0], 0))), 2, true
		}
	case 2:
		if len(rest) >= 4 {
			return RGB(byte8(rest[1][0]), byte8(rest[2][0]), byte8(rest[3][0])), 4, true
		}
	}
	return 0, len(rest), false
}

func byte8(n int) uint8 { return uint8(min(max(n, 0), 255)) }

// ─── Printing ───────────────────────────────────────────────────────────────

var lineDrawing = map[rune]rune{
	'`': '◆', 'a': '▒', 'f': '°', 'g': '±', 'j': '┘', 'k': '┐', 'l': '┌',
	'm': '└', 'n': '┼', 'o': '⎺', 'p': '⎻', 'q': '─', 'r': '⎼', 's': '⎽',
	't': '├', 'u': '┤', 'v': '┴', 'w': '┬', 'x': '│', 'y': '≤', 'z': '≥',
	'{': 'π', '|': '≠', '}': '£', '~': '·',
}

func (t *Terminal) print(r rune) {
	c := &t.cur
	if c.g[c.gl] {
		if d, ok := lineDrawing[r]; ok {
			r = d
		}
	}
	w := 1
	if r >= 0x80 {
		w = RuneWidth(r)
	}
	if w == 0 {
		t.combine(r)
		return
	}
	t.last = r

	if c.wrapNext {
		c.x = 0
		t.index()
	}
	if w == 2 && c.x == t.cols-1 {
		if t.cols < 2 {
			return
		}
		if t.autowrap {
			t.fill(c.y, c.x, c.x+1)
			c.x = 0
			t.index()
		} else {
			c.x--
		}
	}
	if t.insert {
		t.insertCells(w)
	}
	line := t.lines[c.y]
	t.unsplitWide(line, c.x)
	if w == 2 {
		t.unsplitWide(line, c.x+1)
		line[c.x] = Cell{R: r, Wide: WideHead, Style: c.pen}
		line[c.x+1] = Cell{Wide: WideTail, Style: c.pen}
	} else {
		line[c.x] = Cell{R: r, Style: c.pen}
	}
	if c.x+w >= t.cols {
		c.x = t.cols - 1
		c.wrapNext = t.autowrap
	} else {
		c.x += w
	}
}

// combine attaches a zero-width character to the previously printed cell.
func (t *Terminal) combine(r rune) {
	x, y := t.cur.x, t.cur.y
	if !t.cur.wrapNext {
		x--
	}
	if x >= 0 && t.lines[y][x].Wide == WideTail {
		x--
	}
	if x >= 0 && t.lines[y][x].R != 0 && len(t.lines[y][x].Comb) < 32 {
		t.lines[y][x].Comb += string(r)
	}
}

// unsplitWide blanks both halves of a wide character when either half at x
// is about to be overwritten.
func (t *Terminal) unsplitWide(line []Cell, x int) {
	switch line[x].Wide {
	case WideHead:
		if x+1 < len(line) {
			line[x+1] = Cell{Style: line[x+1].Style}
		}
	case WideTail:
		if x > 0 {
			line[x-1] = Cell{Style: line[x-1].Style}
		}
	}
	line[x].Wide = Narrow
}

// ─── Cursor movement ────────────────────────────────────────────────────────

func (t *Terminal) topLimit() int {
	if t.cur.y >= t.top {
		return t.top
	}
	return 0
}

func (t *Terminal) botLimit() int {
	if t.cur.y <= t.bot {
		return t.bot
	}
	return t.rows - 1
}

func (t *Terminal) moveTo(x, y int) {
	t.cur.x = min(max(x, 0), t.cols-1)
	t.cur.y = min(max(y, 0), t.rows-1)
	t.cur.wrapNext = false
}

// moveAbs moves to an absolute position, relative to the scroll region in
// origin mode.
func (t *Terminal) moveAbs(x, y int) {
	if t.cur.origin {
		t.moveTo(x, min(y+t.top, t.bot))
		return
	}
	t.moveTo(x, y)
}

func (t *Terminal) tab(n int) {
	x := t.cur.x
	for ; n > 0 && x < t.cols-1; n-- {
		for x++; x < t.cols-1 && !t.tabs[x]; x++ {
		}
	}
	for ; n < 0 && x > 0; n++ {
		for x--; x > 0 && !t.tabs[x]; x-- {
		}
	}
	t.cur.x = x
	t.cur.wrapNext = false
}

func (t *Terminal) index() {
	t.cur.wrapNext = false
	if t.cur.y == t.bot {
		t.scrollUp(t.top, t.bot, 1)
	} else if t.cur.y < t.rows-1 {
		t.cur.y++
	}
}

func (t *Terminal) reverseIndex() {
	t.cur.wrapNext = false
	if t.cur.y == t.top {
		t.scrollDown(t.top, t.bot, 1)
	} else if t.cur.y > 0 {
		t.cur.y--
	}
}

// ─── Editing ────────────────────────────────────────────────────────────────

// blank is an erased cell: it keeps the pen's background (BCE), like xterm.
func (t *Terminal) blank() Cell { return Cell{Style: Style{Bg: t.cur.pen.Bg}} }

// scrollUp moves rows top+n..bot up n rows, blanking the bottom n. Rows are
// rotated, not copied, so this is O(rows) whatever the width.
func (t *Terminal) scrollUp(top, bot, n int) {
	n = min(n, bot-top+1)
	if n <= 0 {
		return
	}
	region := t.lines[top : bot+1]
	if top == 0 && !t.altScreen {
		for _, l := range region[:n] {
			t.pushHistory(l)
		}
	}
	rotate(region, n)
	for _, l := range region[len(region)-n:] {
		fillCells(l, t.blank())
	}
}

func (t *Terminal) scrollDown(top, bot, n int) {
	n = min(n, bot-top+1)
	if n <= 0 {
		return
	}
	region := t.lines[top : bot+1]
	rotate(region, len(region)-n)
	for _, l := range region[:n] {
		fillCells(l, t.blank())
	}
}

// rotate shifts s left by n, wrapping around.
func rotate(s [][]Cell, n int) {
	if n == 1 { // the common case: one line of output scrolled
		first := s[0]
		copy(s, s[1:])
		s[len(s)-1] = first
		return
	}
	tmp := append([][]Cell(nil), s[:n]...)
	copy(s, s[n:])
	copy(s[len(s)-n:], tmp)
}

func fillCells(l []Cell, c Cell) {
	for i := range l {
		l[i] = c
	}
}

// fill blanks columns [x0, x1) of row y.
func (t *Terminal) fill(y, x0, x1 int) {
	line := t.lines[y]
	x0, x1 = max(x0, 0), min(x1, t.cols)
	if x0 >= x1 {
		return
	}
	t.unsplitWide(line, x0)
	t.unsplitWide(line, x1-1)
	fillCells(line[x0:x1], t.blank())
}

func (t *Terminal) eraseDisplay(mode int) {
	c := &t.cur
	switch mode {
	case 0:
		t.fill(c.y, c.x, t.cols)
		for y := c.y + 1; y < t.rows; y++ {
			t.fill(y, 0, t.cols)
		}
	case 1:
		for y := 0; y < c.y; y++ {
			t.fill(y, 0, t.cols)
		}
		t.fill(c.y, 0, c.x+1)
	case 2:
		for y := 0; y < t.rows; y++ {
			t.fill(y, 0, t.cols)
		}
	case 3: // xterm: erase the scrollback only
		t.ClearHistory()
	}
	c.wrapNext = false
}

func (t *Terminal) eraseLine(mode int) {
	c := &t.cur
	switch mode {
	case 0:
		t.fill(c.y, c.x, t.cols)
	case 1:
		t.fill(c.y, 0, c.x+1)
	case 2:
		t.fill(c.y, 0, t.cols)
	}
	c.wrapNext = false
}

func (t *Terminal) insertLines(n int) {
	if t.cur.y < t.top || t.cur.y > t.bot {
		return
	}
	t.scrollDown(t.cur.y, t.bot, n)
	t.cur.x, t.cur.wrapNext = 0, false
}

func (t *Terminal) deleteLines(n int) {
	if t.cur.y < t.top || t.cur.y > t.bot {
		return
	}
	t.scrollUp(t.cur.y, t.bot, n)
	t.cur.x, t.cur.wrapNext = 0, false
}

func (t *Terminal) insertCells(n int) {
	c := &t.cur
	line := t.lines[c.y]
	n = min(n, t.cols-c.x)
	t.unsplitWide(line, c.x)
	t.unsplitWide(line, t.cols-1-n+1)
	copy(line[c.x+n:], line[c.x:t.cols-n])
	fillCells(line[c.x:c.x+n], t.blank())
	c.wrapNext = false
}

func (t *Terminal) deleteCells(n int) {
	c := &t.cur
	line := t.lines[c.y]
	n = min(n, t.cols-c.x)
	t.unsplitWide(line, c.x)
	if c.x+n < t.cols {
		t.unsplitWide(line, c.x+n)
	}
	copy(line[c.x:], line[c.x+n:])
	fillCells(line[t.cols-n:], t.blank())
	c.wrapNext = false
}
