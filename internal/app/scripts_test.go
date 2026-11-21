//go:build !windows

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/ui"
)

func TestShortcutExecutionAndEnv(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "env.txt")
	script := filepath.Join(t.TempDir(), "save.sh")
	body := "#!/bin/sh\n" +
		"printf '%s|%s|%s|%s|%s\\n' \"$SLAT_SESSION\" \"$SLAT_WORKSPACE\" \"$SLAT_TAB_INDEX\" \"$SLAT_PANE\" \"$SLAT_STATE_JSON\" > " + quote(outFile) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	a, _ := start(t, func(cfg *config.Config) {
		cfg.Shortcuts = map[string]string{
			"Alt+1": script,
			"Alt+2": "exit 7",
		}
	})

	// Trigger Alt+1 (\x1b1).
	a.FeedInput([]byte("\x1b1"))

	deadline := time.Now().Add(3 * time.Second)
	var data []byte
	var err error
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(outFile)
		if err == nil && len(data) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || len(data) == 0 {
		t.Fatalf("shortcut script did not write output: err=%v", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), "|", 5)
	if len(parts) != 5 {
		t.Fatalf("unexpected env output: %q", string(data))
	}
	if parts[0] == "" || parts[1] != "main" || parts[2] != "1" || parts[3] != "1" {
		t.Errorf("unexpected SLAT_* values: %v", parts[:4])
	}
	var st scriptState
	if err := json.Unmarshal([]byte(parts[4]), &st); err != nil {
		t.Fatalf("invalid SLAT_STATE_JSON: %v (%q)", err, parts[4])
	}
	if len(st.Workspaces) != 1 || len(st.Workspaces[0].Tabs) != 1 || len(st.Workspaces[0].Tabs[0].Panes) != 1 {
		t.Errorf("unexpected SLAT_STATE_JSON tree: %+v", st)
	}

	// Trigger Alt+2 (failing script) and verify notification.
	a.FeedInput([]byte("\x1b2"))
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		msg := a.message
		a.mu.Unlock()
		if strings.Contains(msg, "shortcut failed") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected shortcut failure notification")
}

func TestStatusScriptsExecutionAndErrors(t *testing.T) {
	a, _ := start(t, func(cfg *config.Config) {
		fb := "{fg=yellow}FB{/}"
		cfg.Status.Right = "{script:ok} {script:fail} {script:slow} "
		cfg.Status.Scripts = map[string]ui.StatusScriptConfig{
			"ok":   {Command: "printf '{fg=red}VAL{/}\\n'", Interval: "50ms", Timeout: "1s"},
			"fail": {Command: "exit 1", Interval: "50ms", Timeout: "1s", Fallback: &fb},
			"slow": {Command: "sleep 2", Interval: "5s", Timeout: "80ms"},
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		okOut := a.scriptOutputs["ok"]
		failOut := a.scriptOutputs["fail"]
		slowOut := a.scriptOutputs["slow"]
		a.mu.Unlock()
		if okOut == "{fg=red}VAL{/}" && failOut == "{fg=yellow}FB{/}" && slowOut == "{fg=red}[err]{/}" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	t.Fatalf("unexpected scriptOutputs: %+v", a.scriptOutputs)
}

func TestIdentifyPanesToggleAndTabActivePaneID(t *testing.T) {
	a, term := start(t, func(cfg *config.Config) {
		cfg.Status.PaneID = " #{id} "
	})

	// Split so we have 2 panes (IDs 1 and 2).
	a.FeedInput([]byte{prefix, 'v'})
	a.FeedInput([]byte{prefix, 'i'})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		screen := term.String()
		if strings.Contains(screen, "#1") && strings.Contains(screen, "#2") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	screen := term.String()
	if !strings.Contains(screen, "#1") || !strings.Contains(screen, "#2") {
		t.Fatalf("expected both #1 and #2 pane badges on screen, got:\n%s", screen)
	}

	a.mu.Lock()
	st := a.statusOfBar(time.Now())
	a.mu.Unlock()
	if st.PaneID != 2 || len(st.TabActivePaneID) != 1 || st.TabActivePaneID[0] != 2 {
		t.Errorf("statusOfBar PaneID=%d TabActivePaneID=%v, want 2 and [2]", st.PaneID, st.TabActivePaneID)
	}

	// Toggle off.
	a.FeedInput([]byte{prefix, 'i'})
	a.mu.Lock()
	shown := a.showPaneIDs
	a.mu.Unlock()
	if shown {
		t.Errorf("expected showPaneIDs=false after second prefix+i")
	}
}

func TestSendTargetAcrossWorkspacesAndTabs(t *testing.T) {
	a, _ := start(t)

	// Create a second tab in workspace 1, then a second workspace ("ws-2").
	a.FeedInput([]byte{prefix, 'c'}) // tab 2, pane ID 2
	a.FeedInput([]byte{prefix, 'W'}) // workspace "ws-2", tab 1, pane ID 3

	// Switch back to workspace 1 ("main").
	a.FeedInput([]byte{prefix, 'w'})

	// Send to active pane in ws-2 by name without switching focus.
	if err := a.SendTarget(PaneTarget{Pane: "active", Workspace: "ws-2"}, []byte("echo ws2\r")); err != nil {
		t.Fatalf("SendTarget ws-2 active: %v", err)
	}
	// Send to pane 1 in workspace "main", tab 1 by index.
	if err := a.SendTarget(PaneTarget{Pane: "1", Workspace: "main", Tab: "1"}, []byte("echo t1\r")); err != nil {
		t.Fatalf("SendTarget main/1/1: %v", err)
	}
	// Pane 1 is NOT in ws-2, so scoping to ws-2 must reject it.
	if err := a.SendTarget(PaneTarget{Pane: "1", Workspace: "ws-2"}, []byte("x")); err == nil {
		t.Errorf("expected error when pane 1 is outside ws-2")
	}
	if _, err := a.PanesIn("ws-2", ""); err != nil {
		t.Errorf("PanesIn(ws-2): %v", err)
	}
}

func TestNavigationKeysAndFocusDirection(t *testing.T) {
	a, _ := start(t)

	// Split vertically -> pane 1 (left) and pane 2 (right, active).
	a.FeedInput([]byte{prefix, 'v'})
	a.mu.Lock()
	a.render()
	if a.manager.ActivePane().ID != 2 {
		a.mu.Unlock()
		t.Fatalf("expected pane 2 active after split, got %d", a.manager.ActivePane().ID)
	}
	a.mu.Unlock()

	// Default navigation.keys = "alt": Alt+h (\x1bh) switches to pane 1.
	a.FeedInput([]byte("\x1bh"))
	a.mu.Lock()
	if a.manager.ActivePane().ID != 1 {
		a.mu.Unlock()
		t.Fatalf("expected pane 1 active after Alt+h, got %d", a.manager.ActivePane().ID)
	}
	a.mu.Unlock()

	// FocusDirection("right") moves back to pane 2.
	if err := a.FocusDirection("right"); err != nil {
		t.Fatalf("FocusDirection(right): %v", err)
	}
	a.mu.Lock()
	if a.manager.ActivePane().ID != 2 {
		a.mu.Unlock()
		t.Fatalf("expected pane 2 active after FocusDirection(right), got %d", a.manager.ActivePane().ID)
	}
	a.mu.Unlock()

	// Ctrl mode (0x08 = Ctrl-H) when configured.
	a.mu.Lock()
	a.cfg.Navigation.Keys = "ctrl"
	a.mu.Unlock()
	a.FeedInput([]byte{0x08})
	a.mu.Lock()
	if a.manager.ActivePane().ID != 1 {
		a.mu.Unlock()
		t.Fatalf("expected pane 1 active after Ctrl-H, got %d", a.manager.ActivePane().ID)
	}
	a.mu.Unlock()
}

func TestDisplayAlertDuration(t *testing.T) {
	a, _ := start(t)
	a.Display("{fg=red}Saved!{/}", 80*time.Millisecond)

	a.mu.Lock()
	st := a.statusOfBar(time.Now())
	a.mu.Unlock()
	if st.Message != "{fg=red}Saved!{/}" {
		t.Fatalf("initial status Message = %q, want {fg=red}Saved!{/}", st.Message)
	}

	time.Sleep(110 * time.Millisecond)
	a.mu.Lock()
	stAfter := a.statusOfBar(time.Now())
	a.mu.Unlock()
	if stAfter.Message != "" {
		t.Errorf("status Message after duration = %q, want empty", stAfter.Message)
	}
}

func TestFocusPaneTargetAcrossWorkspacesAndTabs(t *testing.T) {
	a, _ := start(t)
	a.FeedInput([]byte{prefix, 'c'}) // tab 2, pane 2
	a.FeedInput([]byte{prefix, 'W'}) // ws-2, tab 1, pane 3

	// Focus back to main/1/1 via FocusPaneTarget.
	if err := a.FocusPaneTarget(PaneTarget{Pane: "1", Workspace: "main", Tab: "1"}); err != nil {
		t.Fatalf("FocusPaneTarget(main/1/1): %v", err)
	}
	a.mu.Lock()
	gotID := a.manager.ActivePane().ID
	gotWS := a.manager.ActiveWorkspace().Name
	a.mu.Unlock()
	if gotID != 1 || gotWS != "main" {
		t.Errorf("after FocusPaneTarget(main/1/1): ws=%q pane=%d, want main/1", gotWS, gotID)
	}

	// Focus active pane of ws-2.
	if err := a.FocusPaneTarget(PaneTarget{Pane: "active", Workspace: "ws-2"}); err != nil {
		t.Fatalf("FocusPaneTarget(ws-2/active): %v", err)
	}
	a.mu.Lock()
	gotID = a.manager.ActivePane().ID
	gotWS = a.manager.ActiveWorkspace().Name
	a.mu.Unlock()
	if gotID != 3 || gotWS != "ws-2" {
		t.Errorf("after FocusPaneTarget(ws-2): ws=%q pane=%d, want ws-2/3", gotWS, gotID)
	}

	// SwitchSession sets PopSwitchTarget when attached.
	a.SwitchSession("/tmp/other-session.sock")
	if got := a.PopSwitchTarget(); got != "/tmp/other-session.sock" {
		t.Errorf("PopSwitchTarget = %q, want /tmp/other-session.sock", got)
	}
}
