package ui

import (
	"strings"
	"testing"
)

// row reads a frame row back as text, which is what the user would see.
func row(f *Frame, y int) string {
	var b strings.Builder
	for _, c := range f.Lines[y] {
		if c.R == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(c.R)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func TestStatusFormatLayout(t *testing.T) {
	st := Status{
		Workspace: "main", WorkspaceIndex: 0, WorkspaceCount: 1,
		Tabs: []string{"shell", "build"}, TabAlert: []bool{false, true},
		ActiveTab: 0, PaneIndex: 1, PaneCount: 3,
	}

	// The default format is the bar slat has always drawn.
	f := NewFrame(60, 1)
	DrawStatusBar(f, 0, st, DefaultStatusFormat)
	if got := row(f, 0); !strings.Contains(got, " main │  1:shell") || !strings.Contains(got, "2:build ?") {
		t.Errorf("default bar = %q", got)
	}
	if !strings.Contains(row(f, 0), "pane 2/3") {
		t.Errorf("default bar lost the pane counter: %q", row(f, 0))
	}

	// A rearranged one: no workspace, tabs in brackets, counts on the left.
	custom, err := ParseStatusFormat(map[string]string{
		"left":  "{pane}/{panes} {tabs}",
		"right": "[{tab}] ",
		"tab":   "[{index} {name}{alert}]",
		"alert": "!",
	})
	if err != nil {
		t.Fatal(err)
	}
	f = NewFrame(60, 1)
	DrawStatusBar(f, 0, st, custom)
	got := row(f, 0)
	if !strings.HasPrefix(got, "2/3 [1 shell] [2 build!]") {
		t.Errorf("custom bar = %q", got)
	}
	if !strings.HasSuffix(got, "[1]") {
		t.Errorf("custom right side = %q", got)
	}
}

// The right side is what survives a narrow terminal: losing which pane you
// are in is worse than a truncated tab list.
func TestStatusBarTruncates(t *testing.T) {
	st := Status{Workspace: "a-very-long-workspace-name", Tabs: []string{"one", "two"}, PaneCount: 2}
	f := NewFrame(24, 1)
	DrawStatusBar(f, 0, st, DefaultStatusFormat)
	got := row(f, 0)
	if len([]rune(got)) > 24 {
		t.Errorf("bar is %d columns wide on a 24-column screen: %q", len([]rune(got)), got)
	}
	if !strings.Contains(got, "pane 1/2") {
		t.Errorf("the right side was dropped first: %q", got)
	}
}

func TestParseStatusFormatErrors(t *testing.T) {
	for _, bad := range []map[string]string{
		{"middle": "x"},            // no such part
		{"left": "{workspaec}"},    // typo in a placeholder
		{"right": "{time} {oops}"}, // one good, one not
	} {
		if _, err := ParseStatusFormat(bad); err == nil {
			t.Errorf("ParseStatusFormat(%v) should have failed", bad)
		}
	}
	// A stray brace is text, not an error: someone's bar may contain one.
	if _, err := ParseStatusFormat(map[string]string{"left": "a { b"}); err != nil {
		t.Errorf("a lone brace should be literal: %v", err)
	}
}

func TestBordersParse(t *testing.T) {
	b, err := ParseBorders(map[string]string{"style": "rounded"})
	if err != nil || b.TopLeft != '╭' {
		t.Errorf("rounded = %+v, %v", b, err)
	}
	// A preset with one glyph of your own.
	b, err = ParseBorders(map[string]string{"style": "heavy", "vertical": "┇"})
	if err != nil || b.Vertical != '┇' || b.TopLeft != '┏' {
		t.Errorf("heavy+override = %+v, %v", b, err)
	}
	for _, bad := range []map[string]string{
		{"style": "fancy"}, // no such style
		{"corner": "x"},    // no such glyph
		{"vertical": "ab"}, // two characters
		{"vertical": "🙂"},  // two columns wide: it would tear the layout
		{"vertical": ""},   // nothing
	} {
		if _, err := ParseBorders(bad); err == nil {
			t.Errorf("ParseBorders(%v) should have failed", bad)
		}
	}
}
