package config

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Animation is how slat moves things: how long, along what curve, and with
// what drawn over a pane while it appears. Setting a duration to 0 turns
// that animation off; `animations = false` turns all of them off.
type Animation struct {
	// Split is how long a new pane takes to appear, Move how long two
	// panes take to trade places.
	Split Duration `toml:"split"`
	Move  Duration `toml:"move"`
	// Easing shapes the motion: linear, out-quad, out-cubic or out-back.
	Easing string `toml:"easing"`
	// Reveal is how a new pane appears: curtain or none.
	Reveal string `toml:"reveal"`
	// Glyph is the character the curtain is drawn with.
	Glyph string `toml:"glyph"`

	ease func(float64) float64 `toml:"-"` // compiled Easing
}

func defaultAnimation() Animation {
	a := Animation{
		Split:  Duration(90 * time.Millisecond),
		Move:   Duration(120 * time.Millisecond),
		Easing: "out-cubic",
		Reveal: "curtain",
		Glyph:  "░",
	}
	a.compile() // the built-in names are known good
	return a
}

// easings are the curves, by name. Each maps a progress in [0,1] to an
// eased progress; out-* start fast and settle, which is what makes a short
// animation feel deliberate rather than abrupt.
var easings = map[string]func(float64) float64{
	"linear":    func(t float64) float64 { return t },
	"out-quad":  func(t float64) float64 { return 1 - (1-t)*(1-t) },
	"out-cubic": func(t float64) float64 { return 1 - math.Pow(1-t, 3) },
	"out-back": func(t float64) float64 {
		const c = 1.70158
		return 1 + (c+1)*math.Pow(t-1, 3) + c*math.Pow(t-1, 2)
	},
}

// EasingNames lists the curves, for errors and docs.
func EasingNames() []string {
	names := make([]string, 0, len(easings))
	for n := range easings {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (a *Animation) compile() error {
	name := strings.ToLower(strings.TrimSpace(a.Easing))
	if name == "" {
		name = "out-cubic"
	}
	ease, ok := easings[name]
	if !ok {
		return fmt.Errorf("animation.easing = %q: no such curve (have %s)",
			a.Easing, strings.Join(EasingNames(), ", "))
	}
	a.Easing, a.ease = name, ease

	switch reveal := strings.ToLower(strings.TrimSpace(a.Reveal)); reveal {
	case "", "curtain":
		a.Reveal = "curtain"
	case "none":
		a.Reveal = "none"
	default:
		return fmt.Errorf("animation.reveal = %q: expected curtain or none", a.Reveal)
	}

	if a.Glyph == "" {
		a.Glyph = "░"
	}
	if utf8.RuneCountInString(a.Glyph) != 1 {
		return fmt.Errorf("animation.glyph = %q: must be exactly one character", a.Glyph)
	}
	if a.Split.D() < 0 || a.Move.D() < 0 {
		return fmt.Errorf("animation durations must not be negative")
	}
	if a.Split.D() > time.Second || a.Move.D() > time.Second {
		return fmt.Errorf("animation durations must be 1s or less: a pane you wait for isn't a feature")
	}
	return nil
}

// Ease applies the configured curve to a progress in [0,1].
func (a Animation) Ease(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	case a.ease == nil:
		return t
	}
	return a.ease(t)
}

// RevealGlyph is the curtain's character.
func (a Animation) RevealGlyph() rune {
	for _, r := range a.Glyph {
		return r
	}
	return '░'
}

// mergeAnimation applies the [animation] table, and the older top-level
// `animate` shorthand that set one duration for everything.
func (cfg *Config) mergeAnimation(user *Config, md toml.MetaData) error {
	if md.IsDefined("animate") {
		if user.Animate.D() < 0 {
			return fmt.Errorf("animate = %q: must not be negative", user.Animate.D())
		}
		cfg.Animation.Split, cfg.Animation.Move = user.Animate, user.Animate
	}
	if md.IsDefined("animations") && !user.Animations {
		cfg.Animation.Split, cfg.Animation.Move = 0, 0
	}
	for _, f := range []struct {
		key string
		set func()
	}{
		{"split", func() { cfg.Animation.Split = user.Animation.Split }},
		{"move", func() { cfg.Animation.Move = user.Animation.Move }},
		{"easing", func() { cfg.Animation.Easing = user.Animation.Easing }},
		{"reveal", func() { cfg.Animation.Reveal = user.Animation.Reveal }},
		{"glyph", func() { cfg.Animation.Glyph = user.Animation.Glyph }},
	} {
		if md.IsDefined("animation", f.key) {
			f.set()
		}
	}
	return cfg.Animation.compile()
}
