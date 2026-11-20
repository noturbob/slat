//go:build !windows

package main

import (
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
