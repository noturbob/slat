package vt

import (
	"fmt"
	"strings"
	"testing"
)

func lineText(l []Cell) string {
	var b strings.Builder
	for _, c := range l {
		if c.Wide != WideTail {
			b.WriteString(c.String())
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func TestHistoryKeepsScrolledLines(t *testing.T) {
	term := New(10, 3)
	term.SetScrollback(4)
	for i := 1; i <= 8; i++ {
		if _, err := fmt.Fprintf(term, "line%d\r\n", i); err != nil {
			t.Fatal(err)
		}
	}
	// Screen holds line7, line8 and the empty cursor row; 1-6 scrolled off,
	// of which the newest 4 are kept.
	if term.Pushed() != 6 || term.FirstLine() != 2 {
		t.Fatalf("pushed %d, first %d", term.Pushed(), term.FirstLine())
	}
	var got []string
	for abs := term.FirstLine(); abs < term.Pushed()+2; abs++ {
		got = append(got, lineText(term.LineAt(abs)))
	}
	if want := "line3 line4 line5 line6 line7 line8"; strings.Join(got, " ") != want {
		t.Errorf("history = %q, want %q", strings.Join(got, " "), want)
	}
	if term.LineAt(1) != nil {
		t.Error("evicted line still returned")
	}
	if l := term.LineAt(3); len(l) != len("line4") {
		t.Errorf("history line not trimmed: %d cells", len(l))
	}
}

func TestHistorySkipsAltScreenAndRegions(t *testing.T) {
	term := New(10, 3)
	term.SetScrollback(100)
	if _, err := term.Write([]byte("\x1b[?1049h1\r\n2\r\n3\r\n4\x1b[?1049l")); err != nil {
		t.Fatal(err)
	}
	if _, err := term.Write([]byte("\x1b[2;3r\x1b[3H\r\n\r\n\r\n\x1b[r")); err != nil {
		t.Fatal(err)
	}
	if term.Pushed() != 0 {
		t.Errorf("pushed %d lines from alt screen / scroll region", term.Pushed())
	}
}

func TestClearScrollback(t *testing.T) {
	term := New(10, 2)
	term.SetScrollback(100)
	if _, err := term.Write([]byte("a\r\nb\r\nc\r\n\x1b[3J")); err != nil {
		t.Fatal(err)
	}
	if term.FirstLine() != term.Pushed() {
		t.Error("ESC[3J did not clear history")
	}
	if !strings.Contains(term.String(), "c") {
		t.Error("ESC[3J erased the screen")
	}
}

func TestShrinkPushesToHistory(t *testing.T) {
	term := New(10, 4)
	term.SetScrollback(100)
	if _, err := term.Write([]byte("1\r\n2\r\n3\r\n$ ")); err != nil {
		t.Fatal(err)
	}
	term.Resize(10, 2)
	if got := lineText(term.LineAt(term.FirstLine())); term.Pushed() != 2 || got != "1" {
		t.Errorf("pushed %d, first line %q", term.Pushed(), got)
	}
}

func TestSearch(t *testing.T) {
	term := New(20, 3)
	term.SetScrollback(100)
	if _, err := term.Write([]byte("alpha\r\nError: disk\r\nbeta\r\n世界 found\r\ngamma\r\nerror again")); err != nil {
		t.Fatal(err)
	}
	last := term.Pushed() + 2
	abs, col, w, ok := term.Search("error", last, -1)
	if !ok || lineText(term.LineAt(abs)) != "error again" || col != 0 || w != 5 {
		t.Fatalf("got %d %d %d %v", abs, col, w, ok)
	}
	abs, _, _, ok = term.Search("error", abs-1, -1) // lowercase query: any case
	if !ok || lineText(term.LineAt(abs)) != "Error: disk" {
		t.Fatalf("second match: %q", lineText(term.LineAt(abs)))
	}
	if _, _, _, ok := term.Search("Error again", last, -1); ok {
		t.Error("capitalized query matched case-insensitively")
	}
	abs, col, w, ok = term.Search("found", term.FirstLine(), 1)
	if !ok || col != 5 || w != 5 { // 世界 is 4 columns, then a space
		t.Errorf("wide-char line: col %d width %d ok %v (line %d)", col, w, ok, abs)
	}
}

func BenchmarkHistoryPush1M(b *testing.B) {
	term := New(80, 24)
	term.SetScrollback(1_000_000)
	line := make([]Cell, 80)
	for i := range line {
		line[i] = Cell{R: 'x'}
	}
	// Fill the 1M ring once so we measure steady-state ring overwrite.
	for i := 0; i < 1_000_000; i++ {
		term.pushHistory(line)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		term.pushHistory(line)
	}
}

func BenchmarkHistorySearch1M(b *testing.B) {
	term := New(20, 3)
	term.SetScrollback(1_000_000)
	line := make([]Cell, 16)
	for i := range line {
		line[i] = Cell{R: 'a'}
	}
	for i := 0; i < 1_000_000; i++ {
		term.pushHistory(line)
	}
	from := term.Pushed() + 2
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = term.Search("needle", from, -1)
	}
}
