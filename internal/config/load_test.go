package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, toml string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "slat"), 0o755)
	os.WriteFile(filepath.Join(dir, "slat", "config.toml"), []byte(toml), 0o644)
	return Load()
}

func TestUserBindingTakesKeyFromDefault(t *testing.T) {
	// "s" is swap-pane by default; giving it to split-horizontal must
	// unbind swap-pane rather than leave two commands on one key.
	cfg, err := load(t, "prefix = \"C-a\"\n[keybinds]\nsplit-horizontal = \"s\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrefixByte != 0x01 {
		t.Errorf("prefix byte = %#x, want 0x01", cfg.PrefixByte)
	}
	if cfg.Keybinds["split-horizontal"] != "s" {
		t.Errorf("split-horizontal = %q", cfg.Keybinds["split-horizontal"])
	}
	if k, ok := cfg.Keybinds["swap-pane"]; ok {
		t.Errorf("swap-pane still bound to %q", k)
	}
	if !cfg.StatusBar || cfg.Keybinds["new-tab"] != "c" {
		t.Error("unset options lost their defaults")
	}
}

func TestInvalidConfigsAreRejected(t *testing.T) {
	for toml, want := range map[string]string{
		"prefix = \"Ctrl+S\"\n":                     "invalid prefix",
		"prefix = \"C-[\"\n":                        "Escape key",
		"scrollback = -5\n":                         "between 0 and",
		"[keybinds]\nsplit-sideways = \"v\"\n":      "unknown keybind",
		"[keybinds]\nnew-tab = \"ct\"\n":            "single character",
		"[keybinds]\nnew-tab = \"a\"\nquit = \"a\"": "bound to both",
		"statusbar = false\n":                       "unknown setting",
	} {
		if _, err := load(t, toml); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got error %v, want %q", toml, err, want)
		}
	}
}

func TestMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil || cfg.PrefixByte != 0x13 || len(cfg.Keybinds) != len(defaultKeybinds()) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	example, err := os.ReadFile("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := load(t, string(example)); err != nil {
		t.Fatalf("config.example.toml: %v", err)
	}
}

func TestAgentSettings(t *testing.T) {
	cfg, err := load(t, `
[agent]
settle = "2s"
input_after = "1m"
input_patterns = ['ready\?']
on_idle = "notify-send done"
`)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Agent
	if a.Settle.D() != 2*time.Second || a.InputAfter.D() != time.Minute {
		t.Errorf("durations: %v, %v", a.Settle.D(), a.InputAfter.D())
	}
	if len(a.Patterns) != 1 || !a.Patterns[0].MatchString("all ready?") {
		t.Errorf("patterns = %v", a.Patterns)
	}
	if a.OnIdle != "notify-send done" {
		t.Errorf("on_idle = %q", a.OnIdle)
	}

	// Unset keys keep the defaults, and bad values are refused.
	cfg, err = load(t, "[agent]\nsettle = \"1s\"\n")
	if err != nil || len(cfg.Agent.Patterns) != len(defaultAgent().InputPatterns) {
		t.Errorf("defaults not kept: %v, %v", cfg.Agent, err)
	}
	if _, err := load(t, "[agent]\nsettle = \"-1s\"\n"); err == nil {
		t.Error("a negative settle should be an error")
	}
	if _, err := load(t, "[agent]\ninput_patterns = ['(']\n"); err == nil {
		t.Error("a bad pattern should be an error")
	}
	if _, err := load(t, "[agent]\nsettle_time = \"1s\"\n"); err == nil {
		t.Error("an unknown agent setting should be an error")
	}
}

func TestAnimationSettings(t *testing.T) {
	cfg, err := load(t, `
[animation]
split  = "50ms"
move   = "200ms"
easing = "linear"
reveal = "none"
glyph  = "▓"
`)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Animation
	if a.Split.D() != 50*time.Millisecond || a.Move.D() != 200*time.Millisecond {
		t.Errorf("durations: %v, %v", a.Split.D(), a.Move.D())
	}
	if a.Reveal != "none" || a.RevealGlyph() != '▓' {
		t.Errorf("reveal = %q %q", a.Reveal, a.RevealGlyph())
	}
	// linear leaves progress alone; the default curve does not.
	if got := a.Ease(0.5); got != 0.5 {
		t.Errorf("linear Ease(0.5) = %v", got)
	}
	if got := DefaultConfig().Animation.Ease(0.5); got <= 0.5 {
		t.Errorf("out-cubic Ease(0.5) = %v, want more than half way", got)
	}

	// The old single knob still works, and one switch turns everything off.
	if cfg, err := load(t, `animate = "300ms"`); err != nil ||
		cfg.Animation.Split.D() != 300*time.Millisecond || cfg.Animation.Move.D() != 300*time.Millisecond {
		t.Errorf("animate shorthand: %+v, %v", cfg.Animation, err)
	}
	if cfg, err := load(t, "animations = false"); err != nil ||
		cfg.Animation.Split != 0 || cfg.Animation.Move != 0 {
		t.Errorf("animations = false: %+v, %v", cfg.Animation, err)
	}

	for _, bad := range []string{
		"[animation]\neasing = \"bouncy\"\n",
		"[animation]\nreveal = \"sparkle\"\n",
		"[animation]\nglyph = \"ab\"\n",
		"[animation]\nsplit = \"-1s\"\n",
		"[animation]\nsplit = \"5s\"\n", // long enough to be in the way
		"[animation]\nspin = \"1s\"\n",  // unknown setting
	} {
		if _, err := load(t, bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}
}

func TestBordersAndStatusSettings(t *testing.T) {
	cfg, err := load(t, `
[borders]
style = "rounded"
vertical = "┇"

[status]
left  = " {workspace} {tabs}"
right = "{time} "
tab   = "[{index}]"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Borders["style"] != "rounded" || cfg.Status["tab"] != "[{index}]" {
		t.Errorf("tables not kept: %v %v", cfg.Borders, cfg.Status)
	}

	// Mistakes are refused when slat starts, not drawn oddly later.
	for _, bad := range []string{
		"[borders]\nstyle = \"fancy\"\n",
		"[borders]\nvertical = \"too long\"\n",
		"[borders]\ncorner = \"+\"\n",
		"[status]\nleft = \"{workspaec}\"\n",
		"[status]\nmiddle = \"x\"\n",
	} {
		if _, err := load(t, bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}
}

// Every rice in the gallery must load: a broken example is worse than none.
func TestGalleryRicesLoad(t *testing.T) {
	rices, err := filepath.Glob("../../docs/rices/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(rices) == 0 {
		t.Fatal("no rices found: did docs/rices move?")
	}
	for _, path := range rices {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := load(t, string(body)); err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}
}
