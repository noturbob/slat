package app_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/noturbob/slat/internal/app"

	"github.com/noturbob/slat/internal/session"
)

func TestFuzzyMatchV2(t *testing.T) {
	// Word-prefix matching: matches start of words/segments, rejects loose subsequences.
	if _, ok := FuzzyMatch("fish", "slat main 1:fish pane 1"); !ok {
		t.Error("expected word-prefix match for fish")
	}
	if _, ok := FuzzyMatch("fi", "slat main 1:fish pane 1"); !ok {
		t.Error("expected word-prefix match for fi")
	}
	if _, ok := FuzzyMatch("monkey", "distilled-monkey /tmp/other detached"); !ok {
		t.Error("expected word-prefix match after hyphen")
	}
	if _, ok := FuzzyMatch("fish", "slat f_i_s_h_x"); ok {
		t.Error("loose subsequence should not match word-prefix filter")
	}
	if _, ok := FuzzyMatch("fl", "slat main 1:fish 2:logs"); ok {
		t.Error("cross-word subsequence should not match word-prefix filter")
	}
	if _, ok := FuzzyMatch("ish", "slat main 1:fish"); ok {
		t.Error("mid-word substring should not match without ' exact modifier")
	}

	// Ranking: full-word match outranks partial-word prefix match even when later in text.
	sFull, ok1 := FuzzyMatch("fish", "fish main")
	sFullLate, ok2 := FuzzyMatch("fish", "slat main 1:zsh pane 1 fish")
	sPartialStart, ok3 := FuzzyMatch("fish", "fisherman main 1:zsh")
	if !ok1 || !ok2 || !ok3 {
		t.Fatalf("expected all to match: ok1=%v ok2=%v ok3=%v", ok1, ok2, ok3)
	}
	if sFull <= sFullLate {
		t.Errorf("earlier full-word match %d should outrank later full-word match %d", sFull, sFullLate)
	}
	if sFullLate <= sPartialStart {
		t.Errorf("full-word match %d should outrank partial-word prefix match %d", sFullLate, sPartialStart)
	}

	// Smart-case: uppercase token requires uppercase match.
	if _, ok := FuzzyMatch("Fish", "slat main 1:fish"); ok {
		t.Error("uppercase query should be case-sensitive")
	}
	if _, ok := FuzzyMatch("Fish", "slat main 1:Fish"); !ok {
		t.Error("uppercase query should match uppercase text")
	}

	// Multi-token AND semantics.
	if _, ok := FuzzyMatch("work 2", "slat work 2:nvim pane 1"); !ok {
		t.Error("multi-token AND query should match")
	}
	if _, ok := FuzzyMatch("work missing", "slat work 2:nvim pane 1"); ok {
		t.Error("multi-token AND query with missing token should not match")
	}

	// fzf extended-search operators: ^prefix, suffix$, 'exact, !negate, | OR.
	if _, ok := FuzzyMatch("^slat", "slat work 2:logs"); !ok {
		t.Error("^slat should match start of text")
	}
	if _, ok := FuzzyMatch("^work", "slat work 2:logs"); ok {
		t.Error("^work should not match when slat is at start")
	}
	if _, ok := FuzzyMatch("logs$", "slat work 2:logs"); !ok {
		t.Error("logs$ should match end of text")
	}
	if _, ok := FuzzyMatch("work$", "slat work 2:logs"); ok {
		t.Error("work$ should not match when not at end of text")
	}
	if _, ok := FuzzyMatch("^exact$", "exact"); !ok {
		t.Error("^exact$ should match full text")
	}
	if _, ok := FuzzyMatch("^exact$", "exact-more"); ok {
		t.Error("^exact$ should not match longer text")
	}
	if _, ok := FuzzyMatch("'ish", "slat main 1:fish"); !ok {
		t.Error("'ish should match exact substring mid-word")
	}
	if _, ok := FuzzyMatch("slat !logs", "slat work 2:logs"); ok {
		t.Error("!logs should exclude matching item")
	}
	if _, ok := FuzzyMatch("slat !logs", "slat work 2:nvim"); !ok {
		t.Error("!logs should keep non-matching item")
	}
	if _, ok := FuzzyMatch("!^slat", "slat work"); ok {
		t.Error("!^slat should exclude item starting with slat")
	}
	if _, ok := FuzzyMatch("work | home", "slat home 1:zsh"); !ok {
		t.Error("OR operator should match second branch")
	}
	if _, ok := FuzzyMatch("slat work | home !zsh", "slat home 1:fish"); !ok {
		t.Error("combined AND/OR/negate query should match")
	}
	if _, ok := FuzzyMatch("slat work | home !zsh", "slat home 1:zsh"); ok {
		t.Error("combined AND/OR/negate query should respect negation")
	}

	// Parenthesis grouping.
	if _, ok := FuzzyMatch("(work | home) logs", "slat home 2:logs"); !ok {
		t.Error("parenthesized OR with AND should match")
	}
	if _, ok := FuzzyMatch("(work|home) logs", "slat work 2:logs"); !ok {
		t.Error("compact parenthesized OR without spaces should match")
	}
	if _, ok := FuzzyMatch("(work logs) | (home zsh)", "slat home 1:zsh"); !ok {
		t.Error("OR of parenthesized AND groups should match second branch")
	}
	if _, ok := FuzzyMatch("(work logs) | (home zsh)", "slat home 2:logs"); ok {
		t.Error("OR of parenthesized AND groups should not mix branches")
	}
	if _, ok := FuzzyMatch("slat !(work | home)", "slat main 1:fish"); !ok {
		t.Error("negated parenthesized group should keep non-matching item")
	}
	if _, ok := FuzzyMatch("slat !(work | home)", "slat work 1:fish"); ok {
		t.Error("negated parenthesized group should exclude matching item")
	}
	if _, ok := FuzzyMatch("(work | home", "slat home 1:fish"); !ok {
		t.Error("unclosed parenthesis while typing should still match")
	}
	if _, ok := FuzzyMatch("work) logs", "slat work 2:logs"); !ok {
		t.Error("stray closing parenthesis should be ignored gracefully")
	}

	// Escaping special characters: (, ), $, ^, ', !, |, space, \.
	if _, ok := FuzzyMatch(`\(fish\)`, "slat main (fish)"); !ok {
		t.Error("escaped parentheses should match literal parentheses")
	}
	if _, ok := FuzzyMatch(`foo\$`, "slat foo$ bar"); !ok {
		t.Error("escaped $ should match literal $ instead of end anchor")
	}
	if _, ok := FuzzyMatch(`foo\\$`, `slat foo\`); !ok {
		t.Error("escaped backslash before $ should still treat $ as end anchor")
	}
	if _, ok := FuzzyMatch(`\^slat`, "main ^slat"); !ok {
		t.Error("escaped ^ should match literal ^")
	}
	if _, ok := FuzzyMatch(`\!warn`, "slat !warn"); !ok {
		t.Error("escaped ! should match literal !")
	}
	if _, ok := FuzzyMatch(`a\|b`, "slat a|b"); !ok {
		t.Error("escaped | should match literal |")
	}
	if _, ok := FuzzyMatch(`work\ 2:logs`, "slat work 2:logs"); !ok {
		t.Error("escaped space should match literal space inside single token")
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

	var wsName, tabName string
	a.WithManagerForTest(func(m *session.Manager) {
		wsName = m.ActiveWorkspace().Name
		tabName = m.ActiveTab().Name
	})
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

func TestBuildTreeSingleLine(t *testing.T) {
	a, term := start(t)
	waitFor(t, term, "pane 1/1", 1)

	labels := a.BuildTreeLabelsForTest()
	if len(labels) != 1 {
		t.Fatalf("buildTree len = %d, want 1", len(labels))
	}
	lbl := labels[0]
	if !strings.Contains(lbl, "1:") || !strings.Contains(lbl, "pane 1") {
		t.Errorf("expected window and pane on one line, got %q", lbl)
	}
}

func TestTreePickerPageUpPageDown(t *testing.T) {
	a, term := start(t)
	var sessions []SessionSummary
	for i := 1; i <= 25; i++ {
		sessions = append(sessions, SessionSummary{
			Name:   "sess-" + string(rune('a'+i-1)),
			Dir:    "/tmp/" + string(rune('a'+i-1)),
			Tabs:   1,
			Panes:  1,
			Socket: "/tmp/s" + string(rune('a'+i-1)) + ".sock",
		})
	}
	a.SetSessionLister(func() []SessionSummary { return sessions })

	a.FeedInput([]byte{prefix, 'F'})
	waitFor(t, term, "sessions (26/26)", 1)

	// PageDown (\x1b[6~) moves down without closing the modal, then Enter selects a later item.
	a.FeedInput([]byte("\x1b[6~\r"))
	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested after PageDown + Enter")
	}
	if got := a.PopSwitchTarget(); got == "" || got == "/tmp/sa.sock" {
		t.Errorf("PopSwitchTarget after PageDown = %q, expected item beyond first", got)
	}

	// Re-open, PageDown then PageUp (\x1b[5~) twice back to the top (item 0 = current session),
	// then Down (\x1b[B) + Enter selects item 1 (/tmp/sa.sock).
	a.FeedInput([]byte{prefix, 'F'})
	waitFor(t, term, "sessions (26/26)", 1)
	a.FeedInput([]byte("\x1b[6~\x1b[5~\x1b[5~\x1b[B\r"))
	select {
	case <-a.DetachRequested():
	case <-time.After(time.Second):
		t.Fatal("expected DetachRequested after PageUp + Enter")
	}
	if got := a.PopSwitchTarget(); got != "/tmp/sa.sock" {
		t.Errorf("PopSwitchTarget after PageUp to top = %q, want /tmp/sa.sock", got)
	}
}

func TestTreePickerCtrlW(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'W'})
	a.FeedInput([]byte{prefix, '$'})
	a.FeedInput([]byte("\x15work\r"))
	a.FeedInput([]byte{prefix, 'c'})
	a.FeedInput([]byte{prefix, ','})
	a.FeedInput([]byte("\x15logs\r"))
	waitFor(t, term, "2:logs", 1)

	a.FeedInput([]byte{prefix, 'P'})
	waitFor(t, term, "main", 1)

	// Open tree picker, type "work missing", press Ctrl-W (0x17) to clear "missing", then type "logs\r".
	a.FeedInput([]byte{prefix, 'f'})
	waitFor(t, term, "session tree", 1)
	a.FeedInput([]byte("work missing\x17logs\r"))
	waitGone(t, term, "session tree")

	var wsName, tabName string
	a.WithManagerForTest(func(m *session.Manager) {
		wsName = m.ActiveWorkspace().Name
		tabName = m.ActiveTab().Name
	})
	if wsName != "work" || tabName != "logs" {
		t.Errorf("jumped to (%q, %q), want (work, logs)", wsName, tabName)
	}
}
