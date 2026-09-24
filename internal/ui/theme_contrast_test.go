package ui

import (
	"math"
	"testing"

	"github.com/noturbob/slat/internal/vt"
)

// A palette that looks lovely in a screenshot can still be unreadable at
// 12px on a laptop in daylight. These floors are the ones WCAG uses for
// large text: the status bar is short, bold-ish labels rather than prose,
// so they are the right bar to hold a theme to -- and they stop a pretty
// but illegible palette from ever being added.
const (
	minText   = 4.5 // status bar text on its background
	minSecond = 3.4 // the dim half, the accent, a tab chip
)

func TestBuiltInThemesAreReadable(t *testing.T) {
	for _, name := range ThemeNames() {
		th := themes[name]
		for _, c := range []struct {
			what string
			a, b vt.Color
			min  float64
		}{
			{"Fg on Bg", th.Fg, th.Bg, minText},
			{"TabFg on TabBg", th.TabFg, th.TabBg, minText},
			{"TabActiveFg on TabActiveBg", th.TabActiveFg, th.TabActiveBg, minSecond},
			{"Dim on Bg", th.Dim, th.Bg, minSecond},
			{"Accent on Bg", th.Accent, th.Bg, minSecond},
		} {
			if got := contrast(c.a, c.b); got < c.min {
				t.Errorf("theme %q: %s is %.1f:1, want at least %.1f:1", name, c.what, got, c.min)
			}
		}
	}
}

// contrast is the WCAG ratio between two colours, 1:1 (identical) to 21:1.
func contrast(a, b vt.Color) float64 {
	l1, l2 := luminance(a), luminance(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func luminance(c vt.Color) float64 {
	r, g, b := channels(c)
	f := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

// channels turns a colour into 0-255 components, resolving palette indexes
// through the xterm-256 table the way a terminal would.
func channels(c vt.Color) (r, g, b float64) {
	if c>>24 == 2 { // 24-bit
		v := uint32(c)
		return float64((v >> 16) & 0xff), float64((v >> 8) & 0xff), float64(v & 0xff)
	}
	n := int(c & 0xff)
	switch {
	case n < 16:
		base := [16][3]float64{
			{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
			{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
			{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		return base[n][0], base[n][1], base[n][2]
	case n < 232:
		n -= 16
		lv := []float64{0, 95, 135, 175, 215, 255}
		return lv[n/36], lv[n/6%6], lv[n%6]
	}
	v := float64(8 + (n-232)*10)
	return v, v, v
}
