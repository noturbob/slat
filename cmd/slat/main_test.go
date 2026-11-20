//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Unix only: $XDG_RUNTIME_DIR and the sun_path length limit are POSIX
// concerns, and Windows uses its own per-user temporary directory.
func TestSocketPathFitsSunPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("d", 120))
	if p := socketPath(); len(p) >= 104 {
		t.Errorf("socket path %d bytes long: %s", len(p), p)
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if p := socketPath(); !strings.HasPrefix(p, "/run/user/1000/") {
		t.Errorf("short runtime dir not used: %s", p)
	}
}

func TestSocketPathPerDirectory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	d1 := t.TempDir()
	d2 := t.TempDir()
	s1 := socketPathForDir(d1)
	s2 := socketPathForDir(d2)
	if s1 == s2 {
		t.Errorf("distinct directories produced same socket: %s", s1)
	}
	if len(s1) >= 104 || len(s2) >= 104 {
		t.Errorf("socket path too long: %s / %s", s1, s2)
	}
	sock, dir, err := resolveAttach([]string{d1})
	if err != nil || sock != s1 || dir == "" {
		t.Errorf("resolveAttach(%q) = %q, %q, %v; want %q", d1, sock, dir, err, s1)
	}
	if _, _, err := resolveAttach([]string{"nonexistent-session-xyz"}); err == nil {
		t.Error("expected error for unknown session name")
	}
}

func TestResolveSessionPreset(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	sessionsDir := filepath.Join(cfgHome, "slat", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	tomlBody := "session_name = \"ex\"\nstart_directory = \"" + workDir + "\"\n"
	namedPath := filepath.Join(sessionsDir, "slat.session_example.toml")
	if err := os.WriteFile(namedPath, []byte(tomlBody), 0o600); err != nil {
		t.Fatal(err)
	}

	sock, dir, path, err := resolveSessionPreset([]string{"slat.session_example"})
	if err != nil {
		t.Fatalf("resolveSessionPreset(name): %v", err)
	}
	if path != namedPath || dir == "" || sock == "" {
		t.Errorf("resolveSessionPreset(name) = (%q, %q, %q)", sock, dir, path)
	}

	_, _, directPath, err := resolveSessionPreset([]string{namedPath})
	if err != nil || directPath != namedPath {
		t.Errorf("resolveSessionPreset(path) = %q, %v", directPath, err)
	}
}

func TestExtractConfigFlag(t *testing.T) {
	cfg, rest, err := extractConfigFlag([]string{"--config", "examples/configs/cyberpunk.toml", "session", "work"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg) || !strings.HasSuffix(cfg, filepath.Join("examples", "configs", "cyberpunk.toml")) {
		t.Errorf("cfg = %q, want abs path ending in examples/configs/cyberpunk.toml", cfg)
	}
	if strings.Join(rest, " ") != "session work" {
		t.Errorf("rest = %v, want [session work]", rest)
	}

	cfg2, rest2, err := extractConfigFlag([]string{"-c=/tmp/custom.toml", "attach"})
	if err != nil || cfg2 != "/tmp/custom.toml" || strings.Join(rest2, " ") != "attach" {
		t.Errorf("-c= got (%q, %v, %v)", cfg2, rest2, err)
	}

	for _, emptyArgs := range [][]string{{"--config="}, {"-c="}, {"--config", ""}} {
		c, _, err := extractConfigFlag(emptyArgs)
		if err != nil || c != "" {
			t.Errorf("%v: got (%q, %v), want empty configPath", emptyArgs, c, err)
		}
	}

	if _, _, err := extractConfigFlag([]string{"--config"}); err == nil {
		t.Errorf("expected error for missing --config argument")
	}
}
