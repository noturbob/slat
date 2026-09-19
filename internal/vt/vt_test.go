package vt

import (
	"fmt"
	"strings"
	"testing"
)

func screen(t *testing.T, cols, rows int, input string) *Terminal {
	t.Helper()
	term := New(cols, rows)
	term.Write([]byte(input))
	return term
}

func TestScreenContents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cols, rows int
		input      string
		want       string
	}{
		{"text and CRLF", 5, 4, "hello\r\nworld", "hello\nworld\n\n"},
		{"autowrap", 5, 4, "abcdefgh", "abcde\nfgh\n\n"},
		{"no wrap until next char", 5, 4, "abcde\r\nx", "abcde\nx\n\n"},
		{"scroll", 5, 4, "1\r\n2\r\n3\r\n4\r\n5", "2\n3\n4\n5"},
		{"cursor position", 5, 4, "\x1b[3;2HX", "\n\n X\n"},
		{"erase line right", 5, 4, "abcde\x1b[3G\x1b[K", "ab\n\n\n"},
		{"erase display", 5, 4, "abc\r\ndef\x1b[2J", "\n\n\n"},
		{"backspace overwrite", 5, 4, "abc\bX", "abX\n\n\n"},
		{"tab", 12, 3, "a\tb", "a       b\n\n"},
		{"insert chars", 8, 2, "abcd\x1b[2G\x1b[2@X", "aX bcd\n"},
		{"delete chars", 8, 2, "abcdef\x1b[2G\x1b[2P", "adef\n"},
		{"ECH", 8, 2, "abcdef\x1b[2G\x1b[3X", "a   ef\n"},
		{"insert line", 5, 4, "1\r\n2\r\n3\x1b[2H\x1b[L", "1\n\n2\n3"},
		{"delete line", 5, 4, "1\r\n2\r\n3\x1b[1H\x1b[M", "2\n3\n\n"},
		{"scroll region", 5, 4, "\x1b[2;3r\x1b[3H\r\nA\r\nB", "\nA\nB\n"},
		{"reverse index at top", 5, 4, "1\r\n2\x1b[H\x1bM", "\n1\n2\n"},
		{"wide chars", 5, 2, "a世b", "a世b\n"},
		{"wide char wraps whole", 5, 2, "abcd世", "abcd\n世"},
		{"overwrite half of wide", 5, 2, "世\x1b[1GX", "X\n"},
		{"combining mark", 5, 2, "e\u0301!", "e\u0301!\n"},
		{"line drawing charset", 5, 2, "\x1b(0lqk\x1b(B", "┌─┐\n"},
		{"REP", 8, 2, "ab\x1b[3b", "abbbb\n"},
		{"alt screen restores main", 5, 2, "main\x1b[?1049halt\x1b[?1049l", "main\n"},
		{"OSC ignored", 5, 2, "\x1b]0;title\x07ok\x1b]2;t\x1b\\!", "ok!\n"},
		{"DCS ignored", 5, 2, "\x1bP1$r0m\x1b\\ok", "ok\n"},
		{"split UTF-8 waits", 5, 2, "\xe4\xb8", "\n"},
		{"invalid UTF-8", 5, 2, "a\xffb", "a\ufffdb\n"},
		{"CUP clamps", 5, 4, "\x1b[99;99HZ", "\n\n\n    Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := screen(t, tc.cols, tc.rows, tc.input).String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSGR(t *testing.T) {
	for input, want := range map[string]Style{
		"\x1b[1;31m":             {Fg: Indexed(1), Attrs: Bold},
		"\x1b[38;5;200;48;5;17m": {Fg: Indexed(200), Bg: Indexed(17)},
		"\x1b[38;2;1;2;3m":       {Fg: RGB(1, 2, 3)},
		"\x1b[38:2::1:2:3m":      {Fg: RGB(1, 2, 3)},
		"\x1b[48:5:9m":           {Bg: Indexed(9)},
		"\x1b[94;103m":           {Fg: Indexed(12), Bg: Indexed(11)},
		"\x1b[1;4;7m\x1b[22;24m": {Attrs: Reverse},
		"\x1b[31m\x1b[m":         {},
		"\x1b[4:3m":              {Attrs: Underline},
		"\x1b[58;2;1;2;3;1m":     {Attrs: Bold}, // underline color skipped
		"\x1b[>4;1m":             {},            // modifyOtherKeys, not SGR
	} {
		term := screen(t, 5, 1, input+"x")
		if got := term.Cell(0, 0).Style; got != want {
			t.Errorf("%q: got %+v, want %+v", input, got, want)
		}
	}
	// SGR round-trips: a Style's escape sequence parses back to it.
	st := Style{Fg: RGB(9, 8, 7), Bg: Indexed(12), Attrs: Italic | Strike}
	if got := screen(t, 5, 1, st.SGR()+"x").Cell(0, 0).Style; got != st {
		t.Errorf("round trip: got %+v, want %+v", got, st)
	}
}

func TestErasePaintsBackground(t *testing.T) {
	term := screen(t, 4, 2, "\x1b[44m\x1b[2J")
	if got := term.Cell(3, 1).Style.Bg; got != Indexed(4) {
		t.Errorf("erased cell bg = %v, want blue", got)
	}
}

func TestModesAndReplies(t *testing.T) {
	term := screen(t, 10, 5, "\x1b[?1h\x1b[?2004h\x1b[?25l\x1b[5 q\x1b[3;4H\x1b[6n\x1b[c")
	if !term.AppCursor() || !term.BracketedPaste() || term.CursorVisible() || term.CursorStyle() != 5 {
		t.Errorf("modes not tracked")
	}
	if got := string(term.Replies()); got != "\x1b[3;4R\x1b[?62;22c" {
		t.Errorf("replies = %q", got)
	}
	if term.Replies() != nil {
		t.Error("replies not cleared")
	}
	term.Write([]byte("\x1bc"))
	if term.AppCursor() || !term.CursorVisible() {
		t.Error("RIS did not reset modes")
	}
}

func TestResizeKeepsCursorLine(t *testing.T) {
	term := screen(t, 10, 5, "1\r\n2\r\n3\r\n4\r\n$ ")
	term.Resize(6, 3)
	if got := term.String(); got != "3\n4\n$" {
		t.Errorf("after shrink: %q", got)
	}
	if x, y := term.Cursor(); x != 2 || y != 2 {
		t.Errorf("cursor at %d,%d", x, y)
	}
	term.Resize(8, 6)
	term.Write([]byte("ok"))
	if got := term.String(); got != "3\n4\n$ ok\n\n\n" {
		t.Errorf("after grow: %q", got)
	}
}

// Every byte sequence must be survivable: no panics, cursor always on screen.
func TestFuzzDoesNotPanic(t *testing.T) {
	seed := uint32(1)
	next := func() byte {
		seed = seed*1664525 + 1013904223
		return byte(seed >> 24)
	}
	alphabet := []byte("\x1b[;?0123456789HJKmrABCDLMP@X\r\n\b\thello世\xe4\xb8\x96()0Bc7")
	term := New(7, 4)
	for i := 0; i < 200000; i++ {
		term.Write([]byte{alphabet[int(next())%len(alphabet)]})
		if i%5000 == 0 {
			term.Resize(1+int(next())%12, 1+int(next())%6)
		}
		x, y := term.Cursor()
		cols, rows := term.Size()
		if x < 0 || x >= cols || y < 0 || y >= rows {
			t.Fatalf("cursor %d,%d outside %dx%d", x, y, cols, rows)
		}
	}
}

func BenchmarkScrolling(b *testing.B) {
	var sb strings.Builder
	for i := 1; i <= 50000; i++ {
		fmt.Fprintf(&sb, "%d\r\n", i)
	}
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	for range b.N {
		term := New(99, 49)
		term.SetScrollback(2000)
		for off := 0; off < len(data); off += 4096 {
			term.Write(data[off:min(off+4096, len(data))])
		}
	}
}

func BenchmarkColoredText(b *testing.B) {
	var sb strings.Builder
	for i := 1; i <= 20000; i++ {
		fmt.Fprintf(&sb, "\x1b[1;3%dm%d \x1b[38;2;10;200;30mhello 世界 \x1b[0m%s\r\n", i%8, i, strings.Repeat("x", 40))
	}
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	for range b.N {
		term := New(99, 49)
		term.SetScrollback(2000)
		for off := 0; off < len(data); off += 4096 {
			term.Write(data[off:min(off+4096, len(data))])
		}
	}
}
