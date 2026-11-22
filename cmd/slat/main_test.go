//go:build !windows

package main

import (
	"bytes"
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
	sock, dir, err := resolveAttach(d1)
	if err != nil || sock != s1 || dir == "" {
		t.Errorf("resolveAttach(%q) = %q, %q, %v; want %q", d1, sock, dir, err, s1)
	}
	if _, _, unknownErr := resolveAttach("nonexistent-session-xyz"); unknownErr == nil {
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

	sock, dir, path, err := resolveSessionPreset("slat.session_example")
	if err != nil {
		t.Fatalf("resolveSessionPreset(name): %v", err)
	}
	if path != namedPath || dir == "" || sock == "" {
		t.Errorf("resolveSessionPreset(name) = (%q, %q, %q)", sock, dir, path)
	}

	_, _, directPath, err := resolveSessionPreset(namedPath)
	if err != nil || directPath != namedPath {
		t.Errorf("resolveSessionPreset(path) = %q, %v", directPath, err)
	}
}

func TestGlobalConfigFlag(t *testing.T) {
	runWithArgs := func(args ...string) (string, int, string) {
		var gotCfg string
		var exitCode int
		var outBuf, errBuf bytes.Buffer
		deps := defaultDeps()
		deps.continueOnErr = true
		deps.out = &outBuf
		deps.errOut = &errBuf
		deps.exit = func(code int) { exitCode = code }
		deps.resolveAttach = func(target string) (string, string, error) {
			return "/attach.sock", "", nil
		}
		deps.resolvePreset = func(target string) (string, string, string, error) {
			return "/preset.sock", "/work", "/preset.toml", nil
		}
		deps.runClient = func(sock, dir, presetPath, configPath string) error {
			gotCfg = configPath
			return nil
		}
		m, _ := newCLIApp(deps)
		m.Run(args...)
		return gotCfg, exitCode, errBuf.String()
	}

	cfg, code, errText := runWithArgs("session", "work", "--config", "examples/configs/cyberpunk.toml")
	if code != 0 || errText != "" {
		t.Fatalf("unexpected error: code=%d err=%s", code, errText)
	}
	if !filepath.IsAbs(cfg) || !strings.HasSuffix(cfg, filepath.Join("examples", "configs", "cyberpunk.toml")) {
		t.Errorf("cfg = %q, want abs path ending in examples/configs/cyberpunk.toml", cfg)
	}

	cfg2, code2, errText2 := runWithArgs("attach", "-c=/tmp/custom.toml")
	if code2 != 0 || errText2 != "" || cfg2 != "/tmp/custom.toml" {
		t.Errorf("-c= got (%q, %d, %q)", cfg2, code2, errText2)
	}

	for _, emptyArgs := range [][]string{{"--config="}, {"-c="}, {"--config", ""}} {
		c, emptyCode, emptyErr := runWithArgs(emptyArgs...)
		if emptyCode != 0 || emptyErr != "" || c != "" {
			t.Errorf("%v: got (%q, %d, %q), want empty configPath", emptyArgs, c, emptyCode, emptyErr)
		}
	}

	_, codeErr, _ := runWithArgs("--config")
	if codeErr == 0 {
		t.Errorf("expected non-zero exit code for missing --config argument")
	}
}
