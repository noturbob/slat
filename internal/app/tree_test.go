package app

import (
	"testing"
	"time"

	"github.com/noturbob/slat/internal/session"
)

func TestFuzzyMatchV2(t *testing.T) {
	// Consecutive boundary match outranks scattered match.
	sGood, ok1 := fuzzyMatch("fish", "slat main 1:fish pane 1")
	sWeak, ok2 := fuzzyMatch("fish", "slat f_i_s_h_x")
	if !ok1 || !ok2 {
		t.Fatalf("expected both to match: ok1=%v ok2=%v", ok1, ok2)
	}
	if sGood <= sWeak {
		t.Errorf("boundary consecutive match %d should outrank scattered %d", sGood, sWeak)
	}

	// Smart-case: uppercase token requires uppercase match.
	if _, ok := fuzzyMatch("Fish", "slat main 1:fish"); ok {
		t.Error("uppercase query should be case-sensitive")
	}
	if _, ok := fuzzyMatch("Fish", "slat main 1:Fish"); !ok {
		t.Error("uppercase query should match uppercase text")
	}

	// Multi-token query matches across different parts of the path.
	if _, ok := fuzzyMatch("work 2", "slat work 2:nvim pane 1"); !ok {
		t.Error("multi-token query should match")
	}
	if _, ok := fuzzyMatch("work missing", "slat work 2:nvim pane 1"); ok {
		t.Error("multi-token query with missing token should not match")
	}
}

func TestTreePickerJump(t *testing.T) {
	a, term := start(t)
	// Create a second workspace with a custom name, and a second tab.
	a.FeedInput([]byte{prefix, 'W'})
	a.FeedInput([]byte{prefix, '$'})
	a.FeedInput([]byte("\x15work\r"))
	a.FeedInput([]byte{prefix, 'c'})
	a.FeedInput([]byte{prefix, ','})
	a.FeedInput([]byte("\x15logs\r"))
	waitFor(t, term, "2:logs", 1)

	// Switch back to workspace 0.
	a.FeedInput([]byte{prefix, 'P'})
	waitFor(t, term, "main", 1)

	// Open tree picker (<prefix> f), filter for "work logs", and jump.
	a.FeedInput([]byte{prefix, 'f'})
	waitFor(t, term, "session tree", 1)
	a.FeedInput([]byte("work logs\r"))
	waitGone(t, term, "session tree")

	a.mu.Lock()
	wsName := a.manager.ActiveWorkspace().Name
	tabName := a.manager.ActiveTab().Name
	a.mu.Unlock()
	if wsName != "work" || tabName != "logs" {
		t.Errorf("jumped to (%q, %q), want (work, logs)", wsName, tabName)
	}
}

func TestRandomSessionNameDictionaryAndReroll(t *testing.T) {
	prefixes, suffixes := session.SessionDictionaries()
	if len(prefixes) != 40 || len(suffixes) != 40 {
		t.Fatalf("dictionary sizes = (%d, %d), want (40, 40)", len(prefixes), len(suffixes))
	}
	inUse := map[string]bool{}
	for _, p := range prefixes {
		for _, s := range suffixes {
			inUse[p+"-"+s] = true
		}
	}
	delete(inUse, "chaotic-essence")
	got := session.RandomSessionName(inUse)
	if got == "" || inUse[got] {
		t.Errorf("RandomSessionName collided or returned empty: %q", got)
	}
}

func TestSessionPickerSwitch(t *testing.T) {
	a, term := start(t)
	a.SetSessionLister(func() []SessionSummary {
		return []SessionSummary{
			{Name: "distilled-monkey", Dir: "/tmp/other", Tabs: 2, Panes: 3, Socket: "/tmp/other.sock"},
		}
	})

	a.FeedInput([]byte{prefix, 'F'})
	waitFor(t, term, "sessions", 1)
	a.FeedInput([]byte("monkey\r"))

	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested on cross-session switch")
	}
	if got := a.PopSwitchTarget(); got != "/tmp/other.sock" {
		t.Errorf("PopSwitchTarget = %q, want /tmp/other.sock", got)
	}
}

func TestGlobalTreePickerJump(t *testing.T) {
	a, term := start(t)
	var focusedSock string
	var focusedWS, focusedTab, focusedPane int
	a.SetGlobalTreeLister(func() []RemoteTreeNode {
		return []RemoteTreeNode{
			{
				Label:      "    pane 2 (nvim)",
				SearchText: "chaotic-essence main 2:nvim pane 2 nvim",
				Workspace:  0,
				Tab:        1,
				PaneID:     42,
				Socket:     "/tmp/chaotic.sock",
			},
		}
	})
	a.SetRemoteFocuser(func(sock string, wsIdx, tabIdx, paneID int) {
		focusedSock, focusedWS, focusedTab, focusedPane = sock, wsIdx, tabIdx, paneID
	})

	a.FeedInput([]byte{prefix, 'g'})
	waitFor(t, term, "all sessions", 1)
	a.FeedInput([]byte("chaotic nvim\r"))

	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested when jumping to remote pane")
	}
	if focusedSock != "/tmp/chaotic.sock" || focusedWS != 0 || focusedTab != 1 || focusedPane != 42 {
		t.Errorf("remote focus = (%q, %d, %d, %d)", focusedSock, focusedWS, focusedTab, focusedPane)
	}
	if got := a.PopSwitchTarget(); got != "/tmp/chaotic.sock" {
		t.Errorf("PopSwitchTarget = %q, want /tmp/chaotic.sock", got)
	}
}

func TestSwitchNextPrevSession(t *testing.T) {
	a, term := start(t)

	// With no other sessions, next-session notifies and does not detach.
	a.FeedInput([]byte{prefix, ')'})
	waitFor(t, term, "no other sessions", 1)

	// Rename current session to "beta" so sorting puts "alpha" before and "gamma" after.
	a.FeedInput([]byte{prefix, 'S'})
	a.FeedInput([]byte("\x15beta\r")) // Ctrl-U clears existing name
	a.SetSessionLister(func() []SessionSummary {
		return []SessionSummary{
			{Name: "gamma", Dir: "/tmp/g", Socket: "/tmp/gamma.sock"},
			{Name: "alpha", Dir: "/tmp/a", Socket: "/tmp/alpha.sock"},
		}
	})

	// next-session (')') switches from beta -> gamma.
	a.FeedInput([]byte{prefix, ')'})
	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested on next-session")
	}
	if got := a.PopSwitchTarget(); got != "/tmp/gamma.sock" {
		t.Errorf("next-session PopSwitchTarget = %q, want /tmp/gamma.sock", got)
	}

	// prev-session ('(') switches from beta -> alpha.
	a.FeedInput([]byte{prefix, '('})
	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested on prev-session")
	}
	if got := a.PopSwitchTarget(); got != "/tmp/alpha.sock" {
		t.Errorf("prev-session PopSwitchTarget = %q, want /tmp/alpha.sock", got)
	}
}
