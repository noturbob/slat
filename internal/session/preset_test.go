package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noturbob/slat/internal/layout"
	. "github.com/noturbob/slat/internal/session"
)

func TestLoadExamplePreset(t *testing.T) {
	p, err := LoadPreset(filepath.Join("..", "..", "slat.session_example.toml"))
	if err != nil {
		t.Fatalf("LoadPreset(slat.session_example.toml): %v", err)
	}
	if p.SessionName != "sample-session" {
		t.Errorf("SessionName = %q, want %q", p.SessionName, "sample-session")
	}
	if len(p.Tabs) != 2 {
		t.Fatalf("Tabs len = %d, want 2", len(p.Tabs))
	}
	if p.Tabs[0].Name != "doc" || p.Tabs[0].Layout != "stack" {
		t.Errorf("Tabs[0] = %+v, want name=doc layout=stack", p.Tabs[0])
	}
	if len(p.Tabs[0].Panes) != 1 || p.Tabs[0].Panes[0].Cmd != "nvim README.md" {
		t.Errorf("Tabs[0].Panes = %+v", p.Tabs[0].Panes)
	}
	if p.Tabs[1].Name != "editor" || p.Tabs[1].Layout != "" {
		t.Errorf("Tabs[1] = %+v", p.Tabs[1])
	}
	if len(p.Tabs[1].Panes) != 1 || p.Tabs[1].Panes[0].Cmd != "nvim -S session.vim" {
		t.Errorf("Tabs[1].Panes = %+v", p.Tabs[1].Panes)
	}
}

func TestResolvePresetPath(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	got, err := ResolvePresetPath("my-session")
	if err != nil {
		t.Fatalf("ResolvePresetPath: %v", err)
	}
	want := filepath.Join(cfgHome, "slat", "sessions", "my-session.toml")
	if got != want {
		t.Errorf("ResolvePresetPath(name) = %q, want %q", got, want)
	}

	direct := filepath.Join(t.TempDir(), "custom.toml")
	if writeErr := os.WriteFile(direct, []byte("session_name = \"x\"\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	got, err = ResolvePresetPath(direct)
	if err != nil {
		t.Fatalf("ResolvePresetPath(direct): %v", err)
	}
	if got != direct {
		t.Errorf("ResolvePresetPath(direct) = %q, want %q", got, direct)
	}
}

func TestSaveAndLoadPresetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.toml")
	orig := Preset{
		SessionName:    "work",
		StartDirectory: dir,
		Tabs: []PresetTab{
			{
				Name:   "code",
				Layout: "master-left",
				Panes: []PresetPane{
					{Cmd: "nvim", Cwd: dir},
					{Cmd: "git status"},
				},
			},
		},
	}
	if err := SavePreset(path, orig); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	loaded, err := LoadPreset(path)
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	if loaded.SessionName != orig.SessionName || len(loaded.Tabs) != 1 || len(loaded.Tabs[0].Panes) != 2 {
		t.Fatalf("round-trip mismatch: %+v", loaded)
	}
}

func TestLoadPresetRejectsUnknownLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("session_name = \"x\"\n[[tabs]]\nname = \"t\"\nlayout = \"bogus\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPreset(path); err == nil {
		t.Fatal("expected error for unknown layout, got nil")
	}
}

func TestPresetFocusField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "focused.toml")
	tomlData := `session_name = "focused"
[[tabs]]
name = "work"
focus = 2
panes = [
  { cmd = "" },
  { cmd = "" },
  { cmd = "" },
]
`
	if err := os.WriteFile(path, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPreset(path)
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	if p.Tabs[0].Focus != 2 {
		t.Fatalf("Tabs[0].Focus = %d, want 2", p.Tabs[0].Focus)
	}

	m := NewManager("/bin/sh", 100, nil)
	defer func() {
		for _, pn := range m.ActivePanes() {
			pn.Close()
		}
	}()
	if err := m.ApplyPreset(p, layout.Rect{Row: 0, Col: 0, Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	panes := m.ActivePanes()
	if len(panes) != 3 {
		t.Fatalf("panes len = %d, want 3", len(panes))
	}
	if m.ActivePane() != panes[1] {
		t.Errorf("ActivePane = %p, want panes[1] (%p)", m.ActivePane(), panes[1])
	}
	exported := m.ExportPreset()
	if len(exported.Tabs) != 1 || exported.Tabs[0].Focus != 2 {
		t.Errorf("ExportPreset Tabs[0].Focus = %d, want 2", exported.Tabs[0].Focus)
	}

	// Out-of-range focus values must fail LoadPreset.
	for _, bad := range []string{"focus = -1", "focus = 4"} {
		badPath := filepath.Join(dir, "bad-focus.toml")
		content := "session_name = \"x\"\n[[tabs]]\nname = \"t\"\n" + bad + "\npanes = [\"\", \"\"]\n"
		if err := os.WriteFile(badPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPreset(badPath); err == nil {
			t.Errorf("LoadPreset(%s) expected error, got nil", bad)
		}
	}
}

func TestPresetTabAndWorkspaceFocus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tabs-focus.toml")
	tomlData := `session_name = "tabs-focus"
focus = 2

[[tabs]]
name = "one"
panes = [{ cmd = "" }]

[[tabs]]
name = "two"
focus = 2
panes = [{ cmd = "" }, { cmd = "" }]
`
	if err := os.WriteFile(path, []byte(tomlData), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPreset(path)
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	if p.Focus != 2 {
		t.Fatalf("p.Focus = %d, want 2", p.Focus)
	}

	m := NewManager("/bin/sh", 100, nil)
	defer m.Shutdown()
	if err := m.ApplyPreset(p, layout.Rect{Row: 0, Col: 0, Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	if got := m.ActiveWorkspace().ActiveTabIdx; got != 1 {
		t.Fatalf("ActiveTabIdx = %d, want 1", got)
	}
	panes := m.ActivePanes()
	if len(panes) != 2 || m.ActivePane() != panes[1] {
		t.Errorf("ActivePane = %p, want panes[1] (%p)", m.ActivePane(), panes[1])
	}
	exported := m.ExportPreset()
	if exported.Focus != 2 || len(exported.Tabs) != 2 || exported.Tabs[1].Focus != 2 {
		t.Errorf("ExportPreset = %+v, want Focus=2 and Tabs[1].Focus=2", exported)
	}

	// Workspace-level and top-level focus with [[workspaces]].
	wsPath := filepath.Join(dir, "ws-focus.toml")
	wsToml := `session_name = "ws-focus"
focus = 2

[[workspaces]]
name = "first"
[[workspaces.tabs]]
name = "a"
panes = [{ cmd = "" }]

[[workspaces]]
name = "second"
focus = 2
[[workspaces.tabs]]
name = "b"
panes = [{ cmd = "" }]
[[workspaces.tabs]]
name = "c"
focus = 2
panes = [{ cmd = "" }, { cmd = "" }]
`
	if err := os.WriteFile(wsPath, []byte(wsToml), 0o600); err != nil {
		t.Fatal(err)
	}
	wp, err := LoadPreset(wsPath)
	if err != nil {
		t.Fatalf("LoadPreset(ws): %v", err)
	}
	wm := NewManager("/bin/sh", 100, nil)
	defer wm.Shutdown()
	if err := wm.ApplyPreset(wp, layout.Rect{Row: 0, Col: 0, Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("ApplyPreset(ws): %v", err)
	}
	if wm.ActiveWorkspaceIdx != 1 || wm.ActiveWorkspace().ActiveTabIdx != 1 {
		t.Fatalf("ws=%d tab=%d, want 1, 1", wm.ActiveWorkspaceIdx, wm.ActiveWorkspace().ActiveTabIdx)
	}
	wExported := wm.ExportPreset()
	if wExported.Focus != 2 || len(wExported.Workspaces) != 2 || wExported.Workspaces[1].Focus != 2 {
		t.Errorf("ExportPreset(ws) = %+v, want Focus=2 and Workspaces[1].Focus=2", wExported)
	}

	// Out-of-range tab/workspace focus values must fail LoadPreset.
	for _, bad := range []string{
		"session_name = \"x\"\nfocus = 3\n[[tabs]]\nname = \"t\"\n",
		"session_name = \"x\"\nfocus = -1\n[[tabs]]\nname = \"t\"\n",
		"session_name = \"x\"\nfocus = 2\n[[workspaces]]\nname = \"w\"\n[[workspaces.tabs]]\nname = \"t\"\n",
		"session_name = \"x\"\n[[workspaces]]\nname = \"w\"\nfocus = 2\n[[workspaces.tabs]]\nname = \"t\"\n",
	} {
		badPath := filepath.Join(dir, "bad-tab-focus.toml")
		if err := os.WriteFile(badPath, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadPreset(badPath); err == nil {
			t.Errorf("LoadPreset(%q) expected error, got nil", bad)
		}
	}
}
