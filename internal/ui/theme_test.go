package ui_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	. "github.com/noturbob/slat/internal/ui"
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
	if got.Bg != Themes["gruvbox"].Bg {
		t.Errorf("the rest of the preset was lost: %v", got.Bg)
	}

	// No settings at all is the default theme.
	if defTheme, defErr := ParseTheme(nil); defErr != nil || !reflect.DeepEqual(defTheme, Themes["default"]) {
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
	if StyleBar.Bg != vt.DefaultColor || StyleTab.Bg != vt.DefaultColor {
		t.Errorf("applied transparent bg: bar=%v tab=%v", StyleBar.Bg, StyleTab.Bg)
	}

	// Dialog colour overrides.
	ds, err := ParseDialogTheme(DialogTheme{
		Fg: "bright-white", SubFg: "248", HeadFg: "#89b4fa", BorderFg: "cyan", Bg: "#1e1e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	ds.Apply()
	if StyleBoxKey.Fg != vt.Indexed(15) || StyleBoxText.Fg != vt.Indexed(248) ||
		StyleBoxHead.Fg != vt.RGB(0x89, 0xb4, 0xfa) || StyleBox.Fg != vt.Indexed(6) ||
		StyleBox.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("dialog styles = box:%+v head:%+v key:%+v text:%+v", *StyleBox, *StyleBoxHead, *StyleBoxKey, *StyleBoxText)
	}
	if _, badErr := ParseDialogTheme(DialogTheme{Fg: "notacolor"}); badErr == nil {
		t.Error("expected error for invalid dialog.fg")
	}

	fs, err := ParseFinderTheme(FinderTheme{
		Fg: "white", SubFg: "248", HeadFg: "cyan", BorderFg: "blue", Bg: "#1e1e2e", SelFg: "16", SelBg: "yellow",
		MatchFg: "bright-green", MatchBg: "235",
	})
	if err != nil {
		t.Fatal(err)
	}
	fs.Apply()
	if StyleFinderKey.Fg != vt.Indexed(7) || StyleFinderSel.Bg != vt.Indexed(3) || StyleFinder.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("finder styles = %+v %+v", *StyleFinder, *StyleFinderSel)
	}
	if StyleFinderMatch.Fg != vt.Indexed(10) || StyleFinderMatch.Bg != vt.Indexed(235) || StyleFinderSelMatch.Fg != vt.Indexed(10) {
		t.Errorf("finder match styles = %+v %+v", *StyleFinderMatch, *StyleFinderSelMatch)
	}

	icon := " ▶ "
	ss, err := ParseStackTheme(StackTheme{Fg: "250", Bg: "236", ActiveFg: "#1e1e2e", ActiveBg: "#89b4fa", Icon: &icon})
	if err != nil {
		t.Fatal(err)
	}
	ss.Apply()
	if StyleStack.Fg != vt.Indexed(250) || StyleStackActive.Bg != vt.RGB(0x89, 0xb4, 0xfa) {
		t.Errorf("stack styles = %+v %+v", *StyleStack, *StyleStackActive)
	}
	if !ss.HasIcon || ss.Icon != " ▶ " {
		t.Errorf("stack icon = (%v, %q), want (true, ' ▶ ')", ss.HasIcon, ss.Icon)
	}
	Themes["default"].Apply()

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
	for name, th := range Themes {
		for i, c := range []vt.Color{th.Bg, th.Fg, th.Dim, th.Accent, th.Border,
			th.TabBg, th.TabFg, th.TabActiveBg, th.TabActiveFg} {
			if c == 0 {
				t.Errorf("theme %q: colour %d is unset", name, i)
			}
		}
	}
}

func TestThemeVariablesAndExternalThemes(t *testing.T) {
	// background / foreground aliases + custom [theme.vars] + theme.* cross-references.
	th, err := ParseThemeWith(
		map[string]string{
			"name":          "gruvbox",
			"background":    "#11111b",
			"foreground":    "#cdd6f4",
			"accent":        "theme.peach",
			"tab_active_fg": "theme.background",
		},
		map[string]string{
			"peach":     "#fab387",
			"my_accent": "theme.peach",
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if th.Bg != vt.RGB(0x11, 0x11, 0x1b) || th.Fg != vt.RGB(0xcd, 0xd6, 0xf4) {
		t.Errorf("aliases bg/fg = %v / %v", th.Bg, th.Fg)
	}
	if th.Accent != vt.RGB(0xfa, 0xb3, 0x87) || th.TabActiveFg != th.Bg {
		t.Errorf("vars/cross-ref accent=%v tab_active_fg=%v want bg=%v", th.Accent, th.TabActiveFg, th.Bg)
	}
	if c, ok := th.ResolveColor("my_accent"); !ok || c != vt.RGB(0xfa, 0xb3, 0x87) {
		t.Errorf("ResolveColor(my_accent) = %v, %v", c, ok)
	}
	if c, ok := th.ResolveColor("background"); !ok || c != th.Bg {
		t.Errorf("ResolveColor(background) = %v, %v", c, ok)
	}

	// Cycle detection in theme variables.
	if _, cycleErr := ParseThemeWith(
		map[string]string{"accent": "theme.a"},
		map[string]string{"a": "theme.b", "b": "theme.a"},
		nil,
	); cycleErr == nil {
		t.Error("expected cycle error")
	}

	// theme.* references in Dialog, Finder, Stack, Splash, and StatusConfig.
	if ds, dErr := ParseDialogThemeIn(DialogTheme{HeadFg: "theme.peach", Bg: "theme.background"}, th); dErr != nil ||
		ds.HeadFg != vt.RGB(0xfa, 0xb3, 0x87) || ds.Bg != th.Bg {
		t.Errorf("ParseDialogThemeIn = %+v, %v", ds, dErr)
	}
	if fs, fErr := ParseFinderThemeIn(FinderTheme{SelBg: "theme.peach"}, th); fErr != nil ||
		fs.SelBg != vt.RGB(0xfa, 0xb3, 0x87) {
		t.Errorf("ParseFinderThemeIn = %+v, %v", fs, fErr)
	}
	if ss, sErr := ParseStackThemeIn(StackTheme{ActiveBg: "theme.peach"}, th); sErr != nil ||
		ss.ActiveBg != vt.RGB(0xfa, 0xb3, 0x87) {
		t.Errorf("ParseStackThemeIn = %+v, %v", ss, sErr)
	}
	if sp, spErr := ParseSplashIn(nil, "", "", "theme.peach", "theme.foreground", "theme.background", th); spErr != nil ||
		sp.Style.Fg != vt.RGB(0xfa, 0xb3, 0x87) || sp.SubStyle.Fg != th.Fg || sp.Bg != th.Bg {
		t.Errorf("ParseSplashIn = %+v, %v", sp, spErr)
	}

	stCfg := DefaultStatusConfig()
	stCfg.Left = "{fg=theme.background,bg=theme.peach}X{/}"
	stCfg.TabColors = []string{"theme.peach", "theme.foreground"}
	sf, sfErr := ParseStatusConfigIn(stCfg, th)
	if sfErr != nil {
		t.Fatal(sfErr)
	}
	if len(sf.TabColors) != 2 || sf.TabColors[0] != vt.RGB(0xfa, 0xb3, 0x87) || sf.TabColors[1] != th.Fg {
		t.Errorf("TabColors = %+v", sf.TabColors)
	}

	// LoadThemesFromDir: custom theme + overriding a built-in theme.
	dir := t.TempDir()
	customTOML := `
background = "#101010"
foreground = "#eeeeee"
accent     = "theme.mint"

[vars]
mint = "#a6e3a1"
`
	if err := os.WriteFile(filepath.Join(dir, "ocean.toml"), []byte(customTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	userThemes, err := LoadThemesFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ParseThemeWith(map[string]string{"name": "ocean"}, nil, userThemes)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Bg != vt.RGB(0x10, 0x10, 0x10) || loaded.Fg != vt.RGB(0xee, 0xee, 0xee) || loaded.Accent != vt.RGB(0xa6, 0xe3, 0xa1) {
		t.Errorf("loaded user theme = %+v", loaded)
	}

	// ListThemes & ExportThemeTOML.
	listed := ListThemes(userThemes, "ocean")
	var foundOcean bool
	for _, item := range listed {
		if item.Name == "ocean" {
			foundOcean = true
			if item.Source != "user" || !item.Active {
				t.Errorf("ocean ThemeInfo = %+v", item)
			}
		}
	}
	if !foundOcean {
		t.Error("expected ocean in ListThemes")
	}
	rpData, rpName, err := ExportThemeTOML("rose-pine", nil, nil)
	if err != nil || rpName != "rose-pine" || len(rpData) == 0 {
		t.Errorf("ExportThemeTOML(rose-pine) = %d bytes, %q, %v", len(rpData), rpName, err)
	}
	ocData, ocName, err := ExportThemeTOML("ocean", userThemes, nil)
	if err != nil || ocName != "ocean" || !containsAll(string(ocData), "#101010", "[vars]", "mint") {
		t.Errorf("ExportThemeTOML(ocean) = %q, %q, %v", string(ocData), ocName, err)
	}
	if _, _, badErr := ExportThemeTOML("no-such-theme", nil, nil); badErr == nil {
		t.Error("expected error for unknown theme")
	}

	// badge_fg / badge_bg in [theme] and theme.badge_* references.
	badgeTheme, bErr := ParseThemeWith(map[string]string{
		"badge_fg": "theme.background",
		"badge_bg": "#ff5500",
	}, nil, nil)
	if bErr != nil {
		t.Fatalf("ParseThemeWith(badge): %v", bErr)
	}
	if c, ok := badgeTheme.ResolveColor("badge_fg"); !ok || c != badgeTheme.Bg {
		t.Errorf("ResolveColor(badge_fg) = (%v, %v), want %v", c, ok, badgeTheme.Bg)
	}
	if c, ok := badgeTheme.ResolveColor("badge_bg"); !ok || c != vt.RGB(0xff, 0x55, 0x00) {
		t.Errorf("ResolveColor(badge_bg) = (%v, %v), want #ff5500", c, ok)
	}
	badgeTheme.Apply()
	Themes["default"].Apply()
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !reflect.ValueOf(s).IsValid() || !func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}() {
			return false
		}
	}
	return true
}
