package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/noturbob/slat/internal/vt"
)

// Theme is the colour of everything slat draws itself: the status bar, the
// pane borders and the overlays. Pane contents are never restyled — that's
// the program's business.
type Theme struct {
	Bg          vt.Color // status bar and overlay background
	Fg          vt.Color // status bar text
	Dim         vt.Color // secondary text: pane count, workspace count
	Accent      vt.Color // workspace name, focused border, badges, banner
	Border      vt.Color // borders between unfocused panes
	TabBg       vt.Color
	TabFg       vt.Color
	TabActiveBg vt.Color
	TabActiveFg vt.Color
}

// themes are the built-in palettes. "default" is what slat has always
// looked like.
var themes = map[string]Theme{
	"default": {
		Bg: vt.Indexed(235), Fg: vt.Indexed(252), Dim: vt.Indexed(248),
		Accent: vt.Indexed(44), Border: vt.Indexed(240),
		TabBg: vt.Indexed(236), TabFg: vt.Indexed(250),
		TabActiveBg: vt.Indexed(25), TabActiveFg: vt.Indexed(231),
	},
	"gruvbox": {
		Bg: vt.RGB(0x3c, 0x38, 0x36), Fg: vt.RGB(0xeb, 0xdb, 0xb2), Dim: vt.RGB(0xa8, 0x99, 0x84),
		Accent: vt.RGB(0xfa, 0xbd, 0x2f), Border: vt.RGB(0x66, 0x5c, 0x54),
		TabBg: vt.RGB(0x32, 0x30, 0x2f), TabFg: vt.RGB(0xd5, 0xc4, 0xa1),
		TabActiveBg: vt.RGB(0x45, 0x85, 0x88), TabActiveFg: vt.RGB(0xfb, 0xf1, 0xc7),
	},
	"nord": {
		Bg: vt.RGB(0x3b, 0x42, 0x52), Fg: vt.RGB(0xe5, 0xe9, 0xf0), Dim: vt.RGB(0xa7, 0xb1, 0xc2),
		Accent: vt.RGB(0x88, 0xc0, 0xd0), Border: vt.RGB(0x4c, 0x56, 0x6a),
		TabBg: vt.RGB(0x34, 0x3b, 0x48), TabFg: vt.RGB(0xd8, 0xde, 0xe9),
		TabActiveBg: vt.RGB(0x5e, 0x81, 0xac), TabActiveFg: vt.RGB(0xec, 0xef, 0xf4),
	},
	"rose-pine": {
		Bg: vt.RGB(0x1f, 0x1d, 0x2e), Fg: vt.RGB(0xe0, 0xde, 0xf4), Dim: vt.RGB(0x90, 0x8c, 0xaa),
		Accent: vt.RGB(0xeb, 0xbc, 0xba), Border: vt.RGB(0x40, 0x3d, 0x52),
		TabBg: vt.RGB(0x26, 0x23, 0x3a), TabFg: vt.RGB(0xe0, 0xde, 0xf4),
		TabActiveBg: vt.RGB(0x31, 0x74, 0x8f), TabActiveFg: vt.RGB(0xe0, 0xde, 0xf4),
	},
	// For terminals with a light background or no colour worth the name.
	"mono": {
		Bg: vt.Indexed(7), Fg: vt.Indexed(0), Dim: vt.Indexed(8),
		Accent: vt.Indexed(0), Border: vt.Indexed(8),
		TabBg: vt.Indexed(7), TabFg: vt.Indexed(0),
		TabActiveBg: vt.Indexed(0), TabActiveFg: vt.Indexed(7),
	},
}

// ThemeNames lists the built-in palettes, for error messages and docs.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for n := range themes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseTheme builds a theme from the [theme] table: a "name" picking a
// built-in palette, plus any single colour overriding it. Anything it
// can't honour exactly is an error, so a typo in a config is reported
// rather than silently ignored.
func ParseTheme(settings map[string]string) (Theme, error) {
	t := themes["default"]
	if name, ok := settings["name"]; ok {
		preset, known := themes[strings.ToLower(strings.TrimSpace(name))]
		if !known {
			return t, fmt.Errorf("theme.name = %q: no such theme (have %s)",
				name, strings.Join(ThemeNames(), ", "))
		}
		t = preset
	}
	fields := map[string]*vt.Color{
		"bg": &t.Bg, "fg": &t.Fg, "dim": &t.Dim, "accent": &t.Accent,
		"border": &t.Border, "tab_bg": &t.TabBg, "tab_fg": &t.TabFg,
		"tab_active_bg": &t.TabActiveBg, "tab_active_fg": &t.TabActiveFg,
	}
	for key, value := range settings {
		if key == "name" {
			continue
		}
		field, ok := fields[key]
		if !ok {
			return t, fmt.Errorf("unknown setting %q in [theme]", key)
		}
		c, err := ParseColor(value)
		if err != nil {
			return t, fmt.Errorf("theme.%s: %w", key, err)
		}
		*field = c
	}
	return t, nil
}

// Apply makes t the theme everything drawn from now on uses.
func (t Theme) Apply() {
	styleBar = vt.Style{Fg: t.Fg, Bg: t.Bg}
	styleWorkspace = vt.Style{Fg: t.Accent, Bg: t.Bg, Attrs: vt.Bold}
	styleTab = vt.Style{Fg: t.TabFg, Bg: t.TabBg}
	styleTabActive = vt.Style{Fg: t.TabActiveFg, Bg: t.TabActiveBg, Attrs: vt.Bold}
	styleBadge = vt.Style{Fg: t.Bg, Bg: t.Accent, Attrs: vt.Bold}
	styleDim = vt.Style{Fg: t.Dim, Bg: t.Bg}
	styleBorder = vt.Style{Fg: t.Border}
	styleBorderOn = vt.Style{Fg: t.Accent, Attrs: vt.Bold}
	styleBox = vt.Style{Fg: t.Accent, Bg: t.Bg}
	styleBoxKey = vt.Style{Fg: t.Fg, Bg: t.Bg, Attrs: vt.Bold}
	styleBoxText = vt.Style{Fg: t.Fg, Bg: t.Bg}
	styleBoxHead = vt.Style{Fg: t.Accent, Bg: t.Bg, Attrs: vt.Bold}
	styleBanner = vt.Style{Fg: t.Accent, Attrs: vt.Bold}
	styleBannerSub = vt.Style{Fg: t.Dim}
}

// named are the colour names every terminal agrees on.
var named = map[string]uint8{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4,
	"magenta": 5, "cyan": 6, "white": 7,
	"bright-black": 8, "bright-red": 9, "bright-green": 10, "bright-yellow": 11,
	"bright-blue": 12, "bright-magenta": 13, "bright-cyan": 14, "bright-white": 15,
	"grey": 8, "gray": 8,
}

// ParseColor reads a colour written as a name, a 0-255 palette index, or
// #rrggbb.
func ParseColor(s string) (vt.Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := named[s]; ok {
		return vt.Indexed(n), nil
	}
	if hex, ok := strings.CutPrefix(s, "#"); ok {
		if len(hex) != 6 {
			return 0, fmt.Errorf("%q: a hex colour looks like #1a2b3c", s)
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return 0, fmt.Errorf("%q: not a hex colour", s)
		}
		return vt.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return 0, fmt.Errorf("%d: a palette index is 0-255", n)
		}
		return vt.Indexed(uint8(n)), nil
	}
	return 0, fmt.Errorf("%q: expected a name, 0-255 or #rrggbb", s)
}
