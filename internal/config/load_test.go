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
