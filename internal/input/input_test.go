package input

import (
	"bytes"
	"strings"
	"testing"
)

func TestParsePrefix(t *testing.T) {
	tests := []struct {
		in      string
		want    []byte
		name    string
		wantErr string
	}{
		{in: "C-s", want: []byte{0x13}, name: "Ctrl-S"},
		{in: "c-a", want: []byte{0x01}, name: "Ctrl-A"},
		{in: "C-\\", want: []byte{0x1c}, name: `Ctrl-\`},
		{in: "a", want: []byte{'a'}, name: "a"},
		{in: "F13", want: []byte("\x1b[25~"), name: "F13"},
		{in: "f13", want: []byte("\x1b[25~"), name: "F13"},
		{in: "F20", want: []byte("\x1b[34~"), name: "F20"},
		{in: "F21", want: []byte("\x1b[20;2~"), name: "F21"},
		{in: "F24", want: []byte("\x1b[24;2~"), name: "F24"},
		{in: `\e[25~`, want: []byte("\x1b[25~"), name: `\e[25~`},
		{in: `\x1b[25~`, want: []byte("\x1b[25~"), name: `\e[25~`},
		{in: "\x1b[25~", want: []byte("\x1b[25~"), name: `\e[25~`},
		{in: `\eOP`, want: []byte("\x1bOP"), name: `\eOP`},
		{in: "C-[", wantErr: "Escape key"},
		{in: `\e`, wantErr: "Escape key"},
		{in: "\x1b", wantErr: "Escape key"},
		{in: `\x1b`, wantErr: "Escape key"},
		{in: `\x1`, wantErr: "incomplete hex escape"},
		{in: `\xzz`, wantErr: "invalid hex escape"},
		{in: "Ctrl+S", wantErr: "invalid prefix"},
		{in: "F25", wantErr: "invalid prefix"},
	}
	for _, tc := range tests {
		p, err := ParsePrefix(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ParsePrefix(%q) err = %v, want substring %q", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePrefix(%q) unexpected err: %v", tc.in, err)
			continue
		}
		if !bytes.Equal(p.Bytes, tc.want) {
			t.Errorf("ParsePrefix(%q).Bytes = %q, want %q", tc.in, p.Bytes, tc.want)
		}
		if p.Name != tc.name {
			t.Errorf("ParsePrefix(%q).Name = %q, want %q", tc.in, p.Name, tc.name)
		}
	}
}

func TestHandlerMultiBytePrefix(t *testing.T) {
	p, err := ParsePrefix("F13")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(p, map[string]string{"split-vertical": "v"})

	// Non-prefix escape sequence does not match.
	if n := h.MatchPrefix([]byte("\x1b[Av")); n != 0 {
		t.Fatalf("MatchPrefix(arrow) = %d, want 0", n)
	}

	// Canonical F13 matches.
	if n := h.MatchPrefix([]byte("\x1b[25~v")); n != 5 {
		t.Fatalf("MatchPrefix(F13) = %d, want 5", n)
	}
	if act := h.ProcessPrefix(); act != ActionNone || !h.IsPrefixActive() {
		t.Fatalf("first ProcessPrefix = (%v, active=%v), want (ActionNone, true)", act, h.IsPrefixActive())
	}
	if act := h.ProcessByte('v'); act != ActionSplitVertical || h.IsPrefixActive() {
		t.Fatalf("ProcessByte('v') = (%v, active=%v), want (ActionSplitVertical, false)", act, h.IsPrefixActive())
	}

	// Alias (CSI u F13) followed by canonical F13 sends canonical prefix.
	alias := []byte("\x1b[57376u")
	if n := h.MatchPrefix(alias); n != len(alias) {
		t.Fatalf("MatchPrefix(alias) = %d, want %d", n, len(alias))
	}
	h.ProcessPrefix()
	if n := h.MatchPrefix([]byte("\x1b[25~")); n != 5 {
		t.Fatalf("MatchPrefix(second F13) = %d, want 5", n)
	}
	if act := h.ProcessPrefix(); act != ActionSendPrefix || h.IsPrefixActive() {
		t.Fatalf("second ProcessPrefix = (%v, active=%v), want (ActionSendPrefix, false)", act, h.IsPrefixActive())
	}
	if !bytes.Equal(h.PrefixBytes(), []byte("\x1b[25~")) {
		t.Fatalf("PrefixBytes = %q, want \\x1b[25~", h.PrefixBytes())
	}
}

func TestForwardLen(t *testing.T) {
	p, err := ParsePrefix("F13")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(p, nil)

	tests := []struct {
		buf  string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"hello world", 11},
		{"hello\x1b[Aworld", 13},
		{"hello\x1b[A\x1b[25~v", 8},
		{"\x1b[A\x1b[57376uv", 3},
	}
	for _, tc := range tests {
		if got := h.ForwardLen([]byte(tc.buf)); got != tc.want {
			t.Errorf("ForwardLen(%q) = %d, want %d", tc.buf, got, tc.want)
		}
	}
}

func BenchmarkForwardLen(b *testing.B) {
	p, _ := ParsePrefix("F13")
	h := NewHandler(p, nil)
	payload := bytes.Repeat([]byte("abcdefghij"), 400) // 4KB paste without prefix
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		if n := h.ForwardLen(payload); n != len(payload) {
			b.Fatalf("got %d", n)
		}
	}
}
