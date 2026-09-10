package pane

import "testing"

func rc(c *cursorState) (int, int) {
	return c.row, c.col
}

func TestCursorStatePrintAndWrap(t *testing.T) {
	var c cursorState

	c.feed(24, 10, []byte("hello"))
	if r, col := rc(&c); r != 0 || col != 5 {
		t.Fatalf("after 'hello': got row=%d col=%d, want row=0 col=5", r, col)
	}

	// Fill out the rest of the line and one more char to trigger a
	// deferred wrap onto the next row.
	c.feed(24, 10, []byte("wo"))
	if r, col := rc(&c); r != 0 || col != 7 {
		t.Fatalf("after 'wo': got row=%d col=%d, want row=0 col=7", r, col)
	}

	c.feed(24, 10, []byte("rldX"))
	// "rld" fills columns 7,8,9 (the last column), leaving a deferred
	// wrap; "X" then wraps to the next row before printing.
	if r, col := rc(&c); r != 1 || col != 1 {
		t.Fatalf("after wrap: got row=%d col=%d, want row=1 col=1", r, col)
	}
}

func TestCursorStateCRLF(t *testing.T) {
	var c cursorState

	c.feed(24, 80, []byte("abc\r\ndef"))
	if r, col := rc(&c); r != 1 || col != 3 {
		t.Fatalf("after CRLF: got row=%d col=%d, want row=1 col=3", r, col)
	}
}

func TestCursorStateCUP(t *testing.T) {
	var c cursorState

	c.feed(24, 80, []byte("\x1b[5;10H"))
	if r, col := rc(&c); r != 4 || col != 9 {
		t.Fatalf("after CUP(5,10): got row=%d col=%d, want row=4 col=9", r, col)
	}

	// Out-of-bounds targets should clamp instead of corrupting state.
	c.feed(24, 80, []byte("\x1b[100;200H"))
	if r, col := rc(&c); r != 23 || col != 79 {
		t.Fatalf("after out-of-range CUP: got row=%d col=%d, want row=23 col=79", r, col)
	}
}

func TestCursorStateBackspaceAndTab(t *testing.T) {
	var c cursorState

	c.feed(24, 80, []byte("abc\b\b"))
	if r, col := rc(&c); r != 0 || col != 1 {
		t.Fatalf("after backspaces: got row=%d col=%d, want row=0 col=1", r, col)
	}

	c.row, c.col = 0, 0
	c.feed(24, 80, []byte("\t"))
	if r, col := rc(&c); r != 0 || col != 8 {
		t.Fatalf("after tab from col0: got row=%d col=%d, want row=0 col=8", r, col)
	}
}

func TestCursorStateSaveRestore(t *testing.T) {
	var c cursorState

	c.feed(24, 80, []byte("\x1b[3;5H\x1b7"))
	c.feed(24, 80, []byte("hello world"))
	c.feed(24, 80, []byte("\x1b8"))

	if r, col := rc(&c); r != 2 || col != 4 {
		t.Fatalf("after ESC7/ESC8 round trip: got row=%d col=%d, want row=2 col=4", r, col)
	}
}

func TestCursorStateSkipsOSC(t *testing.T) {
	var c cursorState

	// A window-title OSC sequence should be fully swallowed, including its
	// printable-looking payload, with no effect on cursor position.
	c.feed(24, 80, []byte("ab\x1b]0;some title\x07cd"))
	if r, col := rc(&c); r != 0 || col != 4 {
		t.Fatalf("after OSC skip: got row=%d col=%d, want row=0 col=4", r, col)
	}
}

func TestCursorStateClamp(t *testing.T) {
	var c cursorState
	c.row, c.col = 10, 20

	c.clamp(5, 8)
	if r, col := rc(&c); r != 4 || col != 7 {
		t.Fatalf("after clamp(5,8): got row=%d col=%d, want row=4 col=7", r, col)
	}
}
