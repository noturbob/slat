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

func TestThemeSubcommands(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	if err := os.MkdirAll(filepath.Join(cfgHome, "slat", "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgHome, "slat", "config.toml"), []byte("[theme]\nname = \"gruvbox\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (string, string, int) {
		var outBuf, errBuf bytes.Buffer
		var exitCode int
		deps := defaultDeps()
		deps.continueOnErr = true
		deps.out = &outBuf
		deps.errOut = &errBuf
		deps.exit = func(code int) { exitCode = code }
		m, _ := newCLIApp(deps)
		m.Run(args...)
		return outBuf.String(), errBuf.String(), exitCode
	}

	out, errOut, code := run("theme", "ls")
	if code != 0 || errOut != "" {
		t.Fatalf("theme ls failed: code=%d err=%s", code, errOut)
	}
	if !strings.Contains(out, "* gruvbox") || !strings.Contains(out, "rose-pine") || !strings.Contains(out, "slat theme export") {
		t.Errorf("unexpected theme ls output:\n%s", out)
	}

	jsonOut, jsonErr, jsonCode := run("theme", "ls", "--json")
	if jsonCode != 0 || jsonErr != "" || !strings.Contains(jsonOut, `"themes"`) || !strings.Contains(jsonOut, `"rose-pine"`) {
		t.Errorf("unexpected theme ls --json output: code=%d err=%s out=%s", jsonCode, jsonErr, jsonOut)
	}

	// Export named theme to a file in a temp working dir and check placement hint.
	workDir := t.TempDir()
	rpPath := filepath.Join(workDir, "rose-pine.toml")
	expOut, expErr, expCode := run("theme", "export", "rose-pine", "-o", rpPath)
	if expCode != 0 || expErr != "" {
		t.Fatalf("theme export rose-pine failed: code=%d err=%s", expCode, expErr)
	}
	if !strings.Contains(expOut, filepath.Join("slat", "themes", "rose-pine.toml")) {
		t.Errorf("expected placement hint in output, got:\n%s", expOut)
	}
	if body, err := os.ReadFile(rpPath); err != nil || !strings.Contains(string(body), "#1f1d2e") {
		t.Errorf("exported rose-pine.toml unexpected: err=%v body=%s", err, string(body))
	}

	// Overwrite protection without --force, then succeed with --force.
	if _, errStr, failCode := run("theme", "export", "rose-pine", "-o", rpPath); failCode == 0 || !strings.Contains(errStr, "already exists") {
		t.Errorf("expected overwrite refusal, got code=%d err=%q", failCode, errStr)
	}
	if _, forceErr, forceCode := run("theme", "export", "rose-pine", "-o", rpPath, "--force"); forceCode != 0 || forceErr != "" {
		t.Errorf("theme export --force failed: code=%d err=%s", forceCode, forceErr)
	}

	// Export active theme (no name arg) to stdout.
	stdOut, stdErr, stdCode := run("theme", "export", "-o", "-")
	if stdCode != 0 || stdErr != "" || !strings.Contains(stdOut, "#3c3836") {
		t.Errorf("theme export (active) to stdout failed: code=%d err=%s out=%s", stdCode, stdErr, stdOut)
	}

	// `slat theme show` prints named or active theme TOML to stdout.
	showOut, showErr, showCode := run("theme", "show", "rose-pine")
	if showCode != 0 || showErr != "" || !strings.Contains(showOut, "#1f1d2e") {
		t.Errorf("theme show rose-pine failed: code=%d err=%s out=%s", showCode, showErr, showOut)
	}
	showDefOut, showDefErr, showDefCode := run("theme", "show")
	if showDefCode != 0 || showDefErr != "" || !strings.Contains(showDefOut, "#3c3836") {
		t.Errorf("theme show (active) failed: code=%d err=%s out=%s", showDefCode, showDefErr, showDefOut)
	}

	// Completion for `slat theme export s<tab>` and `slat theme show s<tab>`.
	items := completeThemeName("s", "")
	if len(items) != 1 || items[0].Value != "solarized" {
		t.Errorf("completeThemeName(\"s\") = %+v, want [solarized]", items)
	}
	allItems := completeThemeName("", "")
	if len(allItems) < 13 {
		t.Errorf("completeThemeName(\"\") returned %d items, want >= 13", len(allItems))
	}
}
