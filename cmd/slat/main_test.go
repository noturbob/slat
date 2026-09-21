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
