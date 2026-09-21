package ui

import (
	"testing"

	"github.com/noturbob/slat/internal/vt"
)

func TestParseColor(t *testing.T) {
	ok := map[string]vt.Color{
		"red":        vt.Indexed(1),
		"BRIGHT-RED": vt.Indexed(9),
		"  44 ":      vt.Indexed(44),
		"0":          vt.Indexed(0),
		"255":        vt.Indexed(255),
		"#1a2b3c":    vt.RGB(0x1a, 0x2b, 0x3c),
		"#FFFFFF":    vt.RGB(255, 255, 255),
	}
	for in, want := range ok {
		got, err := ParseColor(in)
		if err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "256", "-1", "#abc", "#gggggg", "puce", "1.5"} {
		if c, err := ParseColor(in); err == nil {
			t.Errorf("ParseColor(%q) = %v, want an error", in, c)
		}
	}
}

func TestParseTheme(t *testing.T) {
	// A preset, with one colour of it overridden.
	got, err := ParseTheme(map[string]string{"name": "gruvbox", "accent": "#ff0000"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Accent != vt.RGB(255, 0, 0) {
		t.Errorf("accent = %v", got.Accent)
	}
	if got.Bg != themes["gruvbox"].Bg {
		t.Errorf("the rest of the preset was lost: %v", got.Bg)
	}

	// No settings at all is the default theme.
	if got, err := ParseTheme(nil); err != nil || got != themes["default"] {
		t.Errorf("ParseTheme(nil) = %v, %v", got, err)
	}

	// Typos are errors, not surprises later.
	for _, bad := range []map[string]string{
		{"name": "solarized"},
		{"accnet": "red"},
		{"border": "mauve"},
	} {
		if _, err := ParseTheme(bad); err == nil {
			t.Errorf("ParseTheme(%v) should have failed", bad)
		}
	}
}

// Every built-in theme must set every colour: a zero value would paint
// with whatever the terminal's default happens to be.
func TestBuiltInThemesAreComplete(t *testing.T) {
	for name, th := range themes {
		for i, c := range []vt.Color{th.Bg, th.Fg, th.Dim, th.Accent, th.Border,
			th.TabBg, th.TabFg, th.TabActiveBg, th.TabActiveFg} {
			if c == 0 {
				t.Errorf("theme %q: colour %d is unset", name, i)
			}
		}
	}
}
