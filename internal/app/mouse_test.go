//go:build !windows

package app

import (
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/pane"
)

func TestParseMouse(t *testing.T) {
	cases := []struct {
		in     string
		ok     bool
		button int
		x, y   int
		press  bool
		motion bool
		n      int
	}{
		{in: "\x1b[<0;10;5M", ok: true, button: 0, x: 10, y: 5, press: true, n: 10},
		{in: "\x1b[<0;10;5m", ok: true, button: 0, x: 10, y: 5, press: false, n: 10},
		{in: "\x1b[<64;1;1M", ok: true, button: 64, x: 1, y: 1, press: true, n: 10},
		{in: "\x1b[<65;300;90M", ok: true, button: 65, x: 300, y: 90, press: true, n: 13},
		// a drag: the motion bit is set and must not be read as the button
		{in: "\x1b[<32;7;8M", ok: true, button: 0, x: 7, y: 8, press: true, motion: true, n: 10},
		{in: "\x1b[A", ok: false},         // an arrow key, not a mouse report
		{in: "\x1b[<0;10M", ok: false},    // too few fields
		{in: "\x1b[<0;10;5", ok: false},   // split across reads
		{in: "\x1b[<0;1;2;3M", ok: false}, // too many fields
		{in: "\x1b[<0;x;5M", ok: false},   // not a number
	}
	for _, c := range cases {
		ev, n, ok := parseMouse([]byte(c.in), 0)
		if ok != c.ok {
			t.Fatalf("parseMouse(%q): ok = %v, want %v", c.in, ok, c.ok)
		}
		if !ok {
			continue
		}
		if ev.button != c.button || ev.x != c.x || ev.y != c.y || ev.press != c.press || ev.motion != c.motion || n != c.n {
			t.Errorf("parseMouse(%q) = %+v, n=%d; want button=%d x=%d y=%d press=%v motion=%v n=%d",
				c.in, ev, n, c.button, c.x, c.y, c.press, c.motion, c.n)
		}
	}
}

// The report never starts at the beginning of the buffer in practice: it
// arrives behind whatever else the user typed in the same read.
func TestParseMouseAtOffset(t *testing.T) {
	buf := []byte("abc\x1b[<2;4;9m")
	if _, _, ok := parseMouse(buf, 0); ok {
		t.Fatal("parsed a mouse report from plain text")
	}
	ev, n, ok := parseMouse(buf, 3)
	if !ok || ev.button != 2 || ev.x != 4 || ev.y != 9 || ev.press || n != 9 {
		t.Fatalf("got %+v n=%d ok=%v", ev, n, ok)
	}
}

func TestEncodeMouse(t *testing.T) {
	press := mouseEvent{button: 0, press: true}
	if got, want := string(encodeMouse(press, true, 3, 4)), "\x1b[<0;3;4M"; got != want {
		t.Errorf("SGR press = %q, want %q", got, want)
	}
	release := mouseEvent{button: 0}
	if got, want := string(encodeMouse(release, true, 3, 4)), "\x1b[<0;3;4m"; got != want {
		t.Errorf("SGR release = %q, want %q", got, want)
	}
	// The original encoding offsets by 32 and reports any release as 3.
	if got, want := string(encodeMouse(press, false, 3, 4)), "\x1b[M\x20\x23\x24"; got != want {
		t.Errorf("legacy press = %q, want %q", got, want)
	}
	if got, want := string(encodeMouse(release, false, 3, 4)), "\x1b[M\x23\x23\x24"; got != want {
		t.Errorf("legacy release = %q, want %q", got, want)
	}
	// Past column 223 the old encoding has no byte left to say it with.
	if out := encodeMouse(press, false, 400, 4); out != nil {
		t.Errorf("legacy encoding past 223 = %q, want nil", out)
	}
	if out := encodeMouse(press, true, 400, 4); out == nil {
		t.Error("SGR encoding should have no column limit")
	}
	// Modifiers and drag ride in the button byte.
	drag := mouseEvent{button: 0, press: true, motion: true, shift: true}
	if got, want := string(encodeMouse(drag, true, 1, 1)), "\x1b[<36;1;1M"; got != want {
		t.Errorf("drag+shift = %q, want %q", got, want)
	}
}

// Clicking a pane focuses it, which is the whole point of reading the mouse
// even when nothing in the pane wants it.
func TestClickFocusesPane(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'}) // split left/right
	waitFor(t, term, "slat$", 2)

	panes := activePanes(a)
	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2", len(panes))
	}
	// Focus the one that is not active, by clicking its middle.
	var other = panes[0]
	if other == activePane(a) {
		other = panes[1]
	}
	row, col, rows, cols := other.Rect()
	x, y := col+cols/2+1, row+rows/2+1

	a.FeedInput([]byte("\x1b[<0;" + itoa(x) + ";" + itoa(y) + "M"))
	if got := activePane(a); got != other {
		t.Fatalf("click at (%d,%d) did not focus the pane under it", x, y)
	}
}

// A mouse report must never reach the shell as text.
func TestMouseReportIsNotTypedIntoTheShell(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("\x1b[<0;2;2M\x1b[<0;2;2m"))
	// Give the shell a chance to echo it if it leaked through.
	a.FeedInput([]byte("echo marker\r"))
	waitFor(t, term, "marker", 1)
	if s := term.String(); strings.Contains(s, "[<0;2;2") {
		t.Fatalf("the mouse report reached the shell:\n%s", s)
	}
}

// slat asks the terminal for reports only while the feature is on, and the
// client's exit sequence turns them off again.
func TestMouseModeIsAnnouncedToTheTerminal(t *testing.T) {
	_, term := start(t)
	if raw := term.rawString(); !strings.Contains(raw, "\x1b[?1002h") || !strings.Contains(raw, "\x1b[?1006h") {
		t.Error("slat did not enable mouse reporting on the terminal")
	}
	_, term2 := start(t, func(c *config.Config) { c.Mouse = false })
	if raw := term2.rawString(); strings.Contains(raw, "\x1b[?1002h") {
		t.Error("mouse = false still enabled mouse reporting")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The wheel over a pane whose program wants no mouse scrolls slat's own
// history, which is what a terminal would have done with it.
func TestWheelEntersScrollMode(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("seq 1 200\r"))
	waitFor(t, term, "200", 1)

	a.FeedInput([]byte("\x1b[<64;5;5M")) // wheel up
	if open, _ := scrollState(a); !open {
		t.Fatal("wheel up did not enter scroll mode")
	}
	waitFor(t, term, "SCROLL", 1)

	// and the wheel keeps moving the view once it is open
	_, before := scrollState(a)
	a.FeedInput([]byte("\x1b[<64;5;5M"))
	open, after := scrollState(a)
	if !open || after >= before {
		t.Fatal("a second wheel up did not scroll further back")
	}
	a.FeedInput([]byte("\x1b[<65;5;5M")) // wheel down
	open, back := scrollState(a)
	if !open || back <= before-3 {
		t.Fatal("wheel down did not scroll forward")
	}
}

// Dragging the border between two panes resizes them, which is the first
// thing anyone tries once they notice the mouse works.
func TestDragBorderResizes(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'}) // left | right
	waitFor(t, term, "slat$", 2)

	panes := activePanes(a)
	if len(panes) != 2 {
		t.Fatalf("got %d panes, want 2", len(panes))
	}
	_, _, _, leftCols := panes[0].Rect()
	border := leftCols + 1 // 1-based screen column of the divider

	// Press on the border, drag left, release.
	a.FeedInput([]byte("\x1b[<0;" + itoa(border) + ";5M"))
	a.FeedInput([]byte("\x1b[<32;" + itoa(border-10) + ";5M"))
	a.FeedInput([]byte("\x1b[<0;" + itoa(border-10) + ";5m"))

	narrower := waitCols(t, panes[0], func(c int) bool { return c < leftCols },
		"narrower than %d after dragging left", leftCols)
	// and back the other way
	a.FeedInput([]byte("\x1b[<0;" + itoa(narrower+1) + ";5M"))
	a.FeedInput([]byte("\x1b[<32;" + itoa(narrower+21) + ";5M"))
	a.FeedInput([]byte("\x1b[<0;" + itoa(narrower+21) + ";5m"))
	waitCols(t, panes[0], func(c int) bool { return c > narrower },
		"wider than %d after dragging right", narrower)
}

// A drag keeps the border even when the pointer wanders off it, and lets go
// on release.
func TestDragSurvivesLeavingTheBorder(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)

	panes := activePanes(a)
	_, _, _, leftCols := panes[0].Rect()
	border := leftCols + 1

	a.FeedInput([]byte("\x1b[<0;" + itoa(border) + ";5M"))
	// deep inside the right pane, still dragging
	a.FeedInput([]byte("\x1b[<32;" + itoa(border+15) + ";9M"))
	waitCols(t, panes[0], func(c int) bool { return c > leftCols },
		"wider than %d after dragging into the far pane", leftCols)
	a.FeedInput([]byte("\x1b[<0;" + itoa(border+15) + ";9m")) // release
	if dragging(a) {
		t.Fatal("the drag outlived the button")
	}
	// After release, a plain click in that pane focuses it instead.
	wide, _, _, _ := panes[1].Rect()
	_ = wide
	a.FeedInput([]byte("\x1b[<0;" + itoa(border+15) + ";9M"))
	if got := activePane(a); got != panes[1] {
		t.Fatal("a click after the drag did not focus the pane under it")
	}
}

// Clicking a pane must never start a resize.
func TestClickInsideAPaneIsNotADrag(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte("\x1b[<0;3;3M"))
	if dragging(a) {
		t.Fatal("a click inside a pane started a border drag")
	}
}

func dragging(a *App) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.drag != nil
}

// waitCols waits for a pane to reach a width, since a ratio only becomes a
// size when the render loop next lays the tree out.
func waitCols(t *testing.T, p *pane.Pane, ok func(int) bool, what string, args ...any) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var cols int
	for time.Now().Before(deadline) {
		_, _, _, cols = p.Rect()
		if ok(cols) {
			return cols
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pane is %d columns; wanted "+what, append([]any{cols}, args...)...)
	return cols
}
