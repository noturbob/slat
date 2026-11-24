//go:build !windows

package app_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/noturbob/slat/internal/app"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/vt"
)

func startWithConfigFile(t *testing.T, initialTOML string) (*App, *terminal, string, string) {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")

	slatDir := filepath.Join(xdg, "slat")
	if err := os.MkdirAll(slatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(slatDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(initialTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Splash.Enabled = false
	cfg.Animations = false
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	term := &terminal{emu: vt.New(80, 24)}
	a.SetOutput(term)
	if err := a.Start(80, 24); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Shutdown)
	a.Attach(80, 24)
	return a, term, cfgPath, filepath.Join(slatDir, "themes")
}

func TestConfigReloadValidAndInvalid(t *testing.T) {
	a, term, cfgPath, _ := startWithConfigFile(t, "mouse = true\n")
	waitFor(t, term, "pane 1/1", 1)

	// Valid update: disable mouse and change navigation keys.
	if err := os.WriteFile(cfgPath, []byte("mouse = false\n[navigation]\nkeys = \"ctrl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, term, "config reloaded", 1)

	// Invalid update: bad colour / unknown setting keeps previous config and notifies.
	if err := os.WriteFile(cfgPath, []byte("unknown_key = 123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, term, "config reload failed:", 1)

	// Recover with valid config.
	if err := os.WriteFile(cfgPath, []byte("mouse = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, term, "config reloaded", 1)
	_ = a
}

func TestConfigReloadThemeFile(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("PS1", "$ ")
	t.Setenv("ENV", "")

	slatDir := filepath.Join(xdg, "slat")
	themesDir := filepath.Join(slatDir, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	themePath := filepath.Join(themesDir, "custom-live.toml")
	themeTOML := `background = "#111111"
foreground = "#eeeeee"
accent = "#89b4fa"
`
	if err := os.WriteFile(themePath, []byte(themeTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(slatDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[theme]\nname = \"custom-live\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Splash.Enabled = false
	cfg.Animations = false
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	term := &terminal{emu: vt.New(80, 24)}
	a.SetOutput(term)
	if err := a.Start(80, 24); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Shutdown)
	a.Attach(80, 24)
	waitFor(t, term, "pane 1/1", 1)

	time.Sleep(50 * time.Millisecond)
	updatedTOML := `background = "#101010"
foreground = "#ffffff"
accent = "#a6e3a1"
`
	if err := os.WriteFile(themePath, []byte(updatedTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor(t, term, "config reloaded", 1)
}
