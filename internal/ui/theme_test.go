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
	if defTheme, defErr := ParseTheme(nil); defErr != nil || defTheme != themes["default"] {
		t.Errorf("ParseTheme(nil) = %v, %v", defTheme, defErr)
	}

	// Prefix indicator colours can be customised via name, 0-255, or hex.
	p, err := ParseTheme(map[string]string{
		"prefix_fg":        "cyan",
		"prefix_bg":        "236",
		"prefix_active_fg": "#1e1e2e",
		"prefix_active_bg": "bright-yellow",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.PrefixFg != vt.Indexed(6) || p.PrefixBg != vt.Indexed(236) ||
		p.PrefixActiveFg != vt.RGB(0x1e, 0x1e, 0x2e) || p.PrefixActiveBg != vt.Indexed(11) {
		t.Errorf("prefix colours = %+v", p)
	}

	// status_bg and *_bg accept "" / "transparent" for terminal default bg.
	tr, err := ParseTheme(map[string]string{
		"status_bg": "",
		"tab_bg":    "transparent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.HasStatusBg || tr.StatusBg != vt.DefaultColor || !tr.HasTabBg || tr.TabBg != vt.DefaultColor {
		t.Errorf("transparent bg = %+v", tr)
	}
	tr.Apply()
	if styleBar.Bg != vt.DefaultColor || styleTab.Bg != vt.DefaultColor {
		t.Errorf("applied transparent bg: bar=%v tab=%v", styleBar.Bg, styleTab.Bg)
	}

	// Dialog colour overrides.
	ds, err := ParseDialogTheme(DialogTheme{
		Fg: "bright-white", SubFg: "248", HeadFg: "#89b4fa", BorderFg: "cyan", Bg: "#1e1e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	ds.Apply()
	if styleBoxKey.Fg != vt.Indexed(15) || styleBoxText.Fg != vt.Indexed(248) ||
		styleBoxHead.Fg != vt.RGB(0x89, 0xb4, 0xfa) || styleBox.Fg != vt.Indexed(6) ||
		styleBox.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("dialog styles = box:%+v head:%+v key:%+v text:%+v", styleBox, styleBoxHead, styleBoxKey, styleBoxText)
	}
	if _, badErr := ParseDialogTheme(DialogTheme{Fg: "notacolor"}); badErr == nil {
		t.Error("expected error for invalid dialog.fg")
	}

	fs, err := ParseFinderTheme(FinderTheme{
		Fg: "white", SubFg: "248", HeadFg: "cyan", BorderFg: "blue", Bg: "#1e1e2e", SelFg: "16", SelBg: "yellow",
	})
	if err != nil {
		t.Fatal(err)
	}
	fs.Apply()
	if styleFinderKey.Fg != vt.Indexed(7) || styleFinderSel.Bg != vt.Indexed(3) || styleFinder.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("finder styles = %+v %+v", styleFinder, styleFinderSel)
	}

	ss, err := ParseStackTheme(StackTheme{Fg: "250", Bg: "236", ActiveFg: "#1e1e2e", ActiveBg: "#89b4fa"})
	if err != nil {
		t.Fatal(err)
	}
	ss.Apply()
	if styleStack.Fg != vt.Indexed(250) || styleStackActive.Bg != vt.RGB(0x89, 0xb4, 0xfa) {
		t.Errorf("stack styles = %+v %+v", styleStack, styleStackActive)
	}
	themes["default"].Apply()

	// Typos are errors, not surprises later.
	for _, bad := range []map[string]string{
		{"name": "no-such-theme"},
		{"accnet": "red"},
		{"border": "mauve"},
	} {
		if _, badErr := ParseTheme(bad); badErr == nil {
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
