package ui_test

import (
	"io"
	"strings"
	"testing"

	"github.com/noturbob/slat/internal/layout"
	. "github.com/noturbob/slat/internal/ui"
	"github.com/noturbob/slat/internal/vt"
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
	sq, err := ParseBorders(map[string]string{"style": "square"})
	if err != nil || sq.TopLeft != '▪' || sq.Horizontal != '─' || sq.Vertical != '│' {
		t.Errorf("square = %+v, %v", sq, err)
	}
	defModal, err := ParseBordersWithBase("rounded", nil)
	if err != nil || defModal.TopLeft != '╭' {
		t.Errorf("rounded base default = %+v, %v", defModal, err)
	}
	// Modal borders default to rounded when [borders] is unset.
	mUnset, err := ParseModalBorders(nil, nil)
	if err != nil || mUnset.TopLeft != '╭' {
		t.Errorf("ParseModalBorders(nil, nil) = %+v, %v", mUnset, err)
	}
	// Modal borders inherit from [borders] first, then apply their own overrides.
	mInherit, err := ParseModalBorders(
		map[string]string{"style": "double", "vertical": "┃"},
		map[string]string{"top_left": "◈"},
	)
	if err != nil || mInherit.TopLeft != '◈' || mInherit.TopRight != '╗' || mInherit.Vertical != '┃' || mInherit.Horizontal != '═' {
		t.Errorf("ParseModalBorders inherit+override = %+v, %v", mInherit, err)
	}
	mOverrideStyle, err := ParseModalBorders(
		map[string]string{"style": "double"},
		map[string]string{"style": "square"},
	)
	if err != nil || mOverrideStyle.TopLeft != '▪' || mOverrideStyle.Horizontal != '─' {
		t.Errorf("ParseModalBorders style override = %+v, %v", mOverrideStyle, err)
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
		if _, badErr := ParseBorders(bad); badErr == nil {
			t.Errorf("ParseBorders(%v) should have failed", bad)
		}
	}
}

func TestCustomModalBorders(t *testing.T) {
	mustBorders := func(style string) BorderSet {
		b, err := ParseBorders(map[string]string{"style": style})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	t.Cleanup(func() {
		rounded := mustBorders("rounded")
		rounded.ApplyHelp()
		rounded.ApplyFinder()
		rounded.ApplyDialog()
	})

	mustBorders("double").ApplyHelp()
	mustBorders("square").ApplyFinder()
	mustBorders("heavy").ApplyDialog()

	helpFrame := NewFrame(60, 12)
	DrawHelp(helpFrame, "slat · keys", []HelpEntry{{Desc: "Panes"}, {Key: "x", Desc: "Close"}})
	var helpOut strings.Builder
	for y := 0; y < helpFrame.H; y++ {
		helpOut.WriteString(row(helpFrame, y))
		helpOut.WriteByte('\n')
	}
	if !strings.Contains(helpOut.String(), "╔══") || !strings.Contains(helpOut.String(), "══ Panes") {
		t.Errorf("expected double help borders, got:\n%s", helpOut.String())
	}

	finderFrame := NewFrame(60, 10)
	DrawTreePicker(finderFrame, "tree", "", []TreeRow{{Label: "1:sh", Selected: true}}, 0, 1)
	var finderOut strings.Builder
	for y := 0; y < finderFrame.H; y++ {
		finderOut.WriteString(row(finderFrame, y))
		finderOut.WriteByte('\n')
	}
	if !strings.Contains(finderOut.String(), "▪──") || !strings.Contains(finderOut.String(), "├──") {
		t.Errorf("expected square finder borders, got:\n%s", finderOut.String())
	}

	dlgFrame := NewFrame(60, 12)
	DrawDialog(dlgFrame, "rename", "dev")
	var dlgOut strings.Builder
	for y := 0; y < dlgFrame.H; y++ {
		dlgOut.WriteString(row(dlgFrame, y))
		dlgOut.WriteByte('\n')
	}
	if !strings.Contains(dlgOut.String(), "┏━━") {
		t.Errorf("expected heavy dialog borders, got:\n%s", dlgOut.String())
	}
}

func TestStatusInlineColorsAndGlyphs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge} {fg=pane}{pane_glyph}{/} {pane}/{panes} "
	cfg.Tab = " {fg=tab}🐟{/} {tab_glyph} {index}:{name}{alert} "
	cfg.Alert = "{fg=bright-red,bold}!{/}"
	cfg.Prefix = "{fg=cyan,bg=236}  {/}"
	cfg.PrefixActive = "{fg=#1e1e2e,bg=#f9e2af,bold}   {/}"
	cfg.TabColors = []string{"red", "#00ff00"}
	cfg.PaneColors = []string{"208", "44"}
	cfg.TabGlyphs = []string{"◆", "◇"}
	cfg.PaneGlyphs = []string{"①", "②"}

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	st := Status{
		Workspace: "dev",
		Tabs:      []string{"sh", "vim", "log"},
		TabAlert:  []bool{false, true, false},
		ActiveTab: 1,
		PaneIndex: 1,
		PaneCount: 2,
		Badge:     "PREFIX",
	}

	// Idle prefix ("  " in cyan on 236).
	fr := NewFrame(80, 1)
	DrawStatusBar(fr, 0, st, f)
	if got := fr.Lines[0][0]; got.Style.Fg != vt.Indexed(6) || got.Style.Bg != vt.Indexed(236) {
		t.Errorf("idle prefix style = %+v, want fg=6 bg=236", got.Style)
	}

	// Active prefix ("   " in #1e1e2e on #f9e2af, bold) and PREFIX badge suppressed.
	st.PrefixActive = true
	fr = NewFrame(80, 1)
	DrawStatusBar(fr, 0, st, f)
	if got := fr.Lines[0][0]; got.Style.Fg != vt.RGB(0x1e, 0x1e, 0x2e) ||
		got.Style.Bg != vt.RGB(0xf9, 0xe2, 0xaf) || got.Style.Attrs&vt.Bold == 0 {
		t.Errorf("active prefix style = %+v", got.Style)
	}
	gotText := row(fr, 0)
	if strings.Contains(gotText, "PREFIX") {
		t.Errorf("PREFIX badge should be suppressed when {prefix} is present: %q", gotText)
	}
	if !strings.Contains(gotText, "② 2/2") {
		t.Errorf("pane glyph missing: %q", gotText)
	}

	// Verify tab 1's fish glyph is red (index 0) and tab 3 wraps around to red (index 2 % 2 == 0).
	fishCols := []int{}
	for x, c := range fr.Lines[0] {
		if c.R == '🐟' {
			fishCols = append(fishCols, x)
		}
	}
	if len(fishCols) != 3 {
		t.Fatalf("expected 3 fish glyphs, got %d in %q", len(fishCols), gotText)
	}
	if fg := fr.Lines[0][fishCols[0]].Style.Fg; fg != vt.Indexed(1) {
		t.Errorf("tab 1 fish fg = %v, want red (1)", fg)
	}
	if fg := fr.Lines[0][fishCols[1]].Style.Fg; fg != vt.RGB(0, 255, 0) {
		t.Errorf("tab 2 fish fg = %v, want #00ff00", fg)
	}
	if fg := fr.Lines[0][fishCols[2]].Style.Fg; fg != vt.Indexed(1) {
		t.Errorf("tab 3 fish fg = %v, want red (1)", fg)
	}
	// Tab 2 is active, so its fish glyph should inherit TabActiveBg.
	if bg := fr.Lines[0][fishCols[1]].Style.Bg; bg != Themes["default"].TabActiveBg {
		t.Errorf("active tab fish bg = %v, want TabActiveBg %v", bg, Themes["default"].TabActiveBg)
	}

	// Verify pane glyph '②' is coloured with pane_colors[1] = 44.
	paneCol := -1
	for x, c := range fr.Lines[0] {
		if c.R == '②' {
			paneCol = x
			break
		}
	}
	if paneCol < 0 || fr.Lines[0][paneCol].Style.Fg != vt.Indexed(44) {
		t.Errorf("pane glyph col=%d style=%+v, want fg=44", paneCol, fr.Lines[0][paneCol].Style)
	}
}

func TestStatusStyleTagErrors(t *testing.T) {
	for _, bad := range []StatusConfig{
		{Left: "{fg=unknown}x{/}"},
		{Left: "{fg=256}x{/}"},
		{Left: "{bg=#123}x{/}"},
		{Left: "{blink}x{/}"},
		{Left: "{/}"},
		{Left: "{fg=red}a{/}{/}"},
		{Zoom: "{fg=nope}z{/}"},
		{TabZoomGlyph: "{/}"},
		{TabGlyphs: []string{"{fg=nope}x{/}"}},
		{TabColors: []string{"not-a-color"}},
		{PaneColors: []string{"300"}},
	} {
		if _, badErr := ParseStatusConfig(bad); badErr == nil {
			t.Errorf("ParseStatusConfig(%+v) should have failed", bad)
		}
	}
}

func TestStatusZoomedTabAndBadge(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Tab = " {tab_glyph} {index}:{name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabColors = []string{"cyan", "yellow"}
	cfg.TabZoomGlyph = "▣"
	cfg.ZoomGlyphs.Tab = true

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	st := Status{
		Workspace: "main",
		Tabs:      []string{"one", "two"},
		ActiveTab: 1,
		PaneCount: 1,
		Zoomed:    true,
		Badge:     "ZOOM",
	}

	fr := NewFrame(60, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	if !strings.Contains(got, "■ 1:one") || !strings.Contains(got, "▣ 2:two") {
		t.Errorf("zoomed tab glyph = %q", got)
	}
	if !strings.Contains(got, "ZOOM") {
		t.Errorf("ZOOM badge should be visible when zoomed on current tab: %q", got)
	}

	// Verify zoomed glyph inherits tab 2's yellow colour.
	for _, c := range fr.Lines[0] {
		if c.R == '▣' {
			if c.Style.Fg != vt.Indexed(3) {
				t.Errorf("zoomed tab glyph fg = %v, want yellow (3)", c.Style.Fg)
			}
			break
		}
	}

	// {zoom} placeholder in right side also works while keeping ZOOM badge visible.
	cfg2 := DefaultStatusConfig()
	cfg2.Right = "{badge}{zoom} {pane}/{panes} "
	cfg2.Zoom = "{fg=yellow,bold} ▣{/}"
	f2, err := ParseStatusConfig(cfg2)
	if err != nil {
		t.Fatal(err)
	}
	fr2 := NewFrame(60, 1)
	DrawStatusBar(fr2, 0, st, f2)
	got2 := row(fr2, 0)
	if !strings.Contains(got2, "ZOOM") || !strings.Contains(got2, "▣ 1/1") {
		t.Errorf("{zoom} placeholder = %q", got2)
	}
}

func TestStatusThreeGlyphTabs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.ZoomGlyphs.Tab = true
	cfg.ZoomGlyphs.Pane = true
	cfg.TabColors = []string{"cyan", "magenta", "yellow"}
	cfg.PaneColors = []string{"208", "44", "#a6e3a1", "red"}

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	st := Status{
		Workspace:    "main",
		Tabs:         []string{"fish", "fish", "fish"},
		TabPaneCount: []int{3, 2, 2},
		ActiveTab:    2,
		PaneIndex:    0,
		PaneCount:    2,
		Zoomed:       true,
		Badge:        "ZOOM",
	}

	fr := NewFrame(80, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	for _, want := range []string{"■ ① ₃ fish", "■ ② ₂ fish", "▣ ③ ❷ fish"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if !strings.Contains(got, "ZOOM") {
		t.Errorf("ZOOM badge should be visible when zoomed on current tab: %q", got)
	}

	// Verify pane_colors[2] = #a6e3a1 on '₃' (3 panes -> 0-based colour index 2),
	// and pane_colors[1] = 44 on '❷' (2 panes -> 0-based colour index 1).
	for _, c := range fr.Lines[0] {
		if c.R == '₃' && c.Style.Fg != vt.RGB(0xa6, 0xe3, 0xa1) {
			t.Errorf("'₃' fg = %v, want #a6e3a1", c.Style.Fg)
		}
		if c.R == '❷' && c.Style.Fg != vt.Indexed(44) {
			t.Errorf("'❷' fg = %v, want 44", c.Style.Fg)
		}
	}
}

func TestStatusMultiDigitGlyphs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = "{pane_glyph}"
	cfg.Tab = " {tab_glyph}{tab_index_glyph}:{pane_glyph} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.ZoomGlyphs.Tab = true
	cfg.ZoomGlyphs.Pane = true

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	tabs := make([]string, 12)
	counts := make([]int, 12)
	for i := range tabs {
		tabs[i] = "t"
		counts[i] = i + 1
	}
	st := Status{
		Tabs:         tabs,
		TabPaneCount: counts,
		ActiveTab:    9, // tab 10 (10 panes, zoomed)
		PaneIndex:    9, // pane 10
		PaneCount:    10,
		Zoomed:       true,
	}

	fr := NewFrame(160, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	for _, want := range []string{"■①:₁", "■⑨:₉", "▣①⓪:❶⓿", "■①②:₁₂"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if !strings.HasSuffix(got, "₁₀") {
		t.Errorf("right {pane_glyph} for pane 10 = %q, want suffix ₁₀", got)
	}
}

func TestStatusSessionAndDialog(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = " {session} │ {tabs}"
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fr := NewFrame(60, 1)
	DrawStatusBar(fr, 0, Status{Session: "work", Tabs: []string{"fish"}}, f)
	if got := row(fr, 0); !strings.Contains(got, " work │") {
		t.Errorf("{session} bar = %q", got)
	}

	dlg := NewFrame(60, 12)
	cx, cy := DrawDialog(dlg, "rename session", "alpha")
	if cy != 5 || cx <= 0 {
		t.Errorf("dialog cursor = (%d, %d)", cx, cy)
	}
	var joined strings.Builder
	for y := 0; y < dlg.H; y++ {
		joined.WriteString(row(dlg, y))
		joined.WriteByte('\n')
	}
	if !strings.Contains(joined.String(), "rename session") || !strings.Contains(joined.String(), "> alpha") ||
		!strings.Contains(joined.String(), "Enter save · Esc cancel") {
		t.Errorf("dialog rendering unexpected:\n%s", joined.String())
	}
}

func TestDrawHelpExpandsWidth(t *testing.T) {
	f := NewFrame(120, 20)
	longDesc := "Move the pane around (hjkl, q exits)"
	entries := []HelpEntry{
		{Desc: "Panes"},
		{Key: "m", Desc: longDesc},
	}
	DrawHelp(f, "slat · keys", entries)
	var b strings.Builder
	for y := 0; y < f.H; y++ {
		b.WriteString(row(f, y))
		b.WriteByte('\n')
	}
	if !strings.Contains(b.String(), longDesc) {
		t.Errorf("expected full description %q without truncation, got:\n%s", longDesc, b.String())
	}
}

func TestDrawTreePickerMatchHighlight(t *testing.T) {
	Themes["default"].Apply()
	fs, err := ParseFinderTheme(FinderTheme{MatchFg: "bright-yellow"})
	if err != nil {
		t.Fatal(err)
	}
	fs.Apply()

	f := NewFrame(60, 10)
	rows := []TreeRow{
		{Label: "slat / work · 2:logs", Selected: true, MatchSpans: [][2]int{{7, 11}}},
		{Label: "slat / main · 1:fish", Selected: false, MatchSpans: [][2]int{{16, 20}}},
	}
	DrawTreePicker(f, "session tree", "work", rows, 0, 2)

	// Locate 'w' of "work" on selected row and 'f' of "fish" on unselected row.
	var gotSelMatch, gotRowMatch bool
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			c := f.Lines[y][x]
			if c.R == 'w' && c.Style.Fg == StyleFinderSelMatch.Fg && c.Style.Bg == StyleFinderSelMatch.Bg {
				gotSelMatch = true
			}
			if c.R == 'f' && c.Style.Fg == StyleFinderMatch.Fg && c.Style.Bg == StyleFinderMatch.Bg {
				gotRowMatch = true
			}
		}
	}
	if !gotSelMatch || !gotRowMatch {
		t.Errorf("match highlight missing: sel=%v row=%v", gotSelMatch, gotRowMatch)
	}
}

func TestStackIconAndCustomBadges(t *testing.T) {
	Themes["default"].Apply()
	icon := " -> "
	ss, err := ParseStackTheme(StackTheme{Icon: &icon})
	if err != nil {
		t.Fatal(err)
	}
	ss.Apply()

	f := NewFrame(30, 4)
	DrawStackHeaders(f, layout.Rect{Row: 0, Col: 0, Rows: 2, Cols: 30}, []StackHeader{
		{Index: 1, Title: "one", Active: true},
		{Index: 2, Title: "two", Active: false},
	})
	if got := row(f, 0); !strings.HasPrefix(got, " -> 1: one") {
		t.Errorf("active stack header = %q, want prefix ' -> 1: one'", got)
	}
	if got := row(f, 1); !strings.HasPrefix(got, "    2: two") {
		t.Errorf("inactive stack header = %q, want 4-space aligned '    2: two'", got)
	}

	customMove := "MOVING"
	emptyZoom := ""
	cfg := DefaultStatusConfig()
	cfg.Badges = BadgesConfig{Move: &customMove, Zoom: &emptyZoom}
	sf, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if sf.Badges.Move != "MOVING" || sf.Badges.Zoom != "" || sf.Badges.Prefix != "PREFIX" {
		t.Errorf("resolved badges = %+v", sf.Badges)
	}
}

func TestDrawSplash(t *testing.T) {
	Themes["default"].Apply()
	sp, err := ParseSplash([]string{"[SLAT]", " /\\ "}, "v1.0", "press ?", "cyan", "208", "#1e1e2e")
	if err != nil {
		t.Fatal(err)
	}
	fr := NewFrame(40, 10)
	DrawSplash(fr, sp)
	var out strings.Builder
	for y := 0; y < fr.H; y++ {
		out.WriteString(row(fr, y))
		out.WriteByte('\n')
	}
	if !strings.Contains(out.String(), "[SLAT]") || !strings.Contains(out.String(), "v1.0") || !strings.Contains(out.String(), "press ?") {
		t.Errorf("splash output = %q", out.String())
	}
	if fr.Lines[0][0].Style.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("splash bg = %v, want #1e1e2e", fr.Lines[0][0].Style.Bg)
	}
	if _, badErr := ParseSplash(nil, "", "", "notacolor", "", ""); badErr == nil {
		t.Error("expected error for invalid splash.fg")
	}
}

func TestStatusPaneGlyphCurrent(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = "{badge}{pane_glyph_current}"
	cfg.Tab = " {tab_index_glyph}:{pane_glyph_current}/{pane_glyph} "
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.ZoomGlyphs.Pane = true
	cfg.PaneColors = []string{"cyan", "yellow", "red"}

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	st := Status{
		Tabs:          []string{"a", "b"},
		TabPaneCount:  []int{3, 2},
		TabActivePane: []int{1, 0}, // tab 1 has pane 2 selected; tab 2 has pane 1 selected
		ActiveTab:     0,
		PaneIndex:     1, // pane 2 selected
		PaneCount:     3,
		Zoomed:        true,
		Badge:         "ZOOM",
	}
	fr := NewFrame(40, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	if !strings.Contains(got, "①:❷/❸") || !strings.Contains(got, "②:₁/₂") {
		t.Errorf("tab pane_glyph_current = %q", got)
	}
	if !strings.HasSuffix(got, "❷") || !strings.Contains(got, "ZOOM") {
		t.Errorf("right pane_glyph_current = %q", got)
	}
}

func TestStatusTabActiveAndSeparator(t *testing.T) {
	Themes["default"].Apply()

	// tab_sep = "" removes spacing between tabs; tab_active overrides tab for ActiveTab.
	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = ""
	cfg.Tab = "[{index}:{name}]"
	cfg.TabActive = "<{index}:{name}*>"
	cfg.TabSep = ""

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	st := Status{
		Tabs:      []string{"one", "two", "three"},
		ActiveTab: 1,
	}
	fr := NewFrame(40, 1)
	DrawStatusBar(fr, 0, st, f)
	if got := row(fr, 0); got != "[1:one]<2:two*>[3:three]" {
		t.Errorf("no-sep + tab_active = %q", got)
	}

	// Custom separator renders only between tabs.
	cfg.TabSep = "{fg=grey}│{/}"
	f2, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fr2 := NewFrame(40, 1)
	DrawStatusBar(fr2, 0, st, f2)
	if got := row(fr2, 0); got != "[1:one]│<2:two*>│[3:three]" {
		t.Errorf("custom tab_sep = %q", got)
	}
}

func BenchmarkDrawStatusBar(b *testing.B) {
	Themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.ZoomGlyphs.Tab = true
	cfg.ZoomGlyphs.Pane = true
	cfg.TabColors = []string{"cyan", "magenta", "yellow"}
	cfg.PaneColors = []string{"208", "44", "#a6e3a1", "red"}
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		b.Fatal(err)
	}
	st := Status{
		Workspace:    "main",
		Tabs:         []string{"fish", "fish", "fish"},
		TabPaneCount: []int{3, 2, 2},
		ActiveTab:    2,
		PaneIndex:    0,
		PaneCount:    2,
		Zoomed:       true,
	}
	fr := NewFrame(120, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawStatusBar(fr, 0, st, f)
	}
}

func BenchmarkDrawStatusBarUncompiled(b *testing.B) {
	Themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.ZoomGlyphs.Tab = true
	cfg.ZoomGlyphs.Pane = true
	cfg.TabColors = []string{"cyan", "magenta", "yellow"}
	cfg.PaneColors = []string{"208", "44", "#a6e3a1", "red"}
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		b.Fatal(err)
	}
	ClearCompiledForBench(&f)
	st := Status{
		Workspace:    "main",
		Tabs:         []string{"fish", "fish", "fish"},
		TabPaneCount: []int{3, 2, 2},
		ActiveTab:    2,
		PaneIndex:    0,
		PaneCount:    2,
		Zoomed:       true,
	}
	fr := NewFrame(120, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DrawStatusBar(fr, 0, st, f)
	}
}

func BenchmarkScreenRender(b *testing.B) {
	sc := NewScreen(io.Discard)
	fr1 := NewFrame(120, 40)
	fr2 := NewFrame(120, 40)
	st1 := vt.Style{Fg: vt.Indexed(252), Bg: vt.Indexed(235)}
	st2 := vt.Style{Fg: vt.RGB(0x89, 0xb4, 0xfa), Bg: vt.RGB(0x1e, 0x1e, 0x2e), Attrs: vt.Bold}
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			st := st1
			if (x+y)%5 == 0 {
				st = st2
			}
			r := 'a' + rune((x+y)%26)
			if x%7 == 0 {
				r = '─'
			}
			fr1.Set(x, y, vt.Cell{R: r, Style: st})
			fr2.Set(x, y, vt.Cell{R: r + 1, Style: st})
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fr := fr1
		if i%2 == 1 {
			fr = fr2
		}
		if err := sc.Render(fr, Cursor{X: 10, Y: 5, Visible: true, Style: 2}, Modes{}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStatusScriptRenderingAndValidation(t *testing.T) {
	Themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Right = "{bg=236}{script:git}{/} {pane}/{panes} "
	fb := "{fg=red}err{/}"
	badFb := "{fg=nope}x{/}"
	cfg.Scripts = map[string]StatusScriptConfig{
		"git": {
			Command:  "echo main",
			Interval: "5s",
			Timeout:  "2s",
			Fallback: &fb,
		},
	}
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatalf("ParseStatusConfig: %v", err)
	}

	fr := NewFrame(80, 1)
	st := Status{
		Workspace: "main",
		Tabs:      []string{"sh"},
		PaneCount: 1,
		Scripts: map[string]string{
			"git": "{fg=red}VAL{/} {literal} {fg=nope}bad{/}\n",
		},
	}
	DrawStatusBar(fr, 0, st, f)

	line := row(fr, 0)
	if !strings.Contains(line, "VAL {literal} {fg=nope}bad") {
		t.Fatalf("rendered line = %q, want VAL with literal fallback for invalid tags", line)
	}
	valCol := strings.Index(line, "VAL")
	if valCol < 0 {
		t.Fatalf("VAL not found in %q", line)
	}
	cell := fr.Lines[0][valCol]
	if cell.Style.Fg != vt.Indexed(1) || cell.Style.Bg != vt.Indexed(236) {
		t.Errorf("VAL style = %+v, want fg=1 bg=236", cell.Style)
	}

	// Validation error cases.
	for _, bad := range []StatusConfig{
		func() StatusConfig {
			c := DefaultStatusConfig()
			c.Right = "{script:missing}"
			return c
		}(),
		func() StatusConfig {
			c := DefaultStatusConfig()
			c.Scripts = map[string]StatusScriptConfig{"bad": {Command: ""}}
			return c
		}(),
		func() StatusConfig {
			c := DefaultStatusConfig()
			c.Scripts = map[string]StatusScriptConfig{"bad": {Command: "echo hi", Interval: "-1s"}}
			return c
		}(),
		func() StatusConfig {
			c := DefaultStatusConfig()
			c.Scripts = map[string]StatusScriptConfig{"bad": {Command: "echo hi", Timeout: "0s"}}
			return c
		}(),
		func() StatusConfig {
			c := DefaultStatusConfig()
			c.Scripts = map[string]StatusScriptConfig{"bad": {Command: "echo hi", Fallback: &badFb}}
			return c
		}(),
	} {
		if _, badErr := ParseStatusConfig(bad); badErr == nil {
			t.Errorf("ParseStatusConfig(%+v) should have failed", bad)
		}
	}
}

func TestCurrentPaneAndIDGlyphsAndPaneIDBadge(t *testing.T) {
	Themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Tab = " {index}:{current_pane_glyph}:{current_pane_id_glyph} "
	cfg.PaneID = "{fg=cyan,bg=236} #{id} {/}"
	cfg.PaneIDActive = "{fg=#1e1e2e,bg=#f9e2af,bold} #{id} {/}"
	cfg.CurrentPaneGlyphs = []string{"a", "b", "c"}
	cfg.CurrentPaneZoomGlyphs = []string{"A", "B", "C"}
	cfg.CurrentPaneIDGlyphs = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	cfg.CurrentPaneIDZoomGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}

	fOff, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatalf("ParseStatusConfig (default off): %v", err)
	}
	if fOff.HasZoomIndicator() {
		t.Errorf("HasZoomIndicator = true, want false when zoom_glyphs are all false by default")
	}

	cfg.ZoomGlyphs.CurrentPane = true
	cfg.ZoomGlyphs.CurrentPaneID = true

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatalf("ParseStatusConfig: %v", err)
	}
	if !f.HasZoomIndicator() {
		t.Errorf("HasZoomIndicator = false, want true when current_pane_zoom_glyphs is set")
	}

	st := Status{
		Workspace:       "main",
		Tabs:            []string{"one", "two"},
		TabPaneCount:    []int{2, 3},
		TabActivePane:   []int{1, 0},
		TabActivePaneID: []int{4, 12},
		ActiveTab:       0,
		PaneID:          4,
		PaneIndex:       1,
		PaneCount:       2,
		Zoomed:          false,
	}
	fr := NewFrame(80, 3)
	DrawStatusBar(fr, 2, st, f)
	got := row(fr, 2)
	if !strings.Contains(got, "1:b:4") || !strings.Contains(got, "2:a:12") {
		t.Fatalf("unzoomed status bar = %q, want 1:b:4 and 2:a:12", got)
	}

	st.Zoomed = true
	frZoom := NewFrame(80, 3)
	DrawStatusBar(frZoom, 2, st, f)
	gotZoom := row(frZoom, 2)
	if !strings.Contains(gotZoom, "1:B:④") || !strings.Contains(gotZoom, "2:a:12") {
		t.Fatalf("zoomed status bar = %q, want 1:B:④ and 2:a:12", gotZoom)
	}

	// Test DrawPaneIDBadge right-alignment and theming.
	DrawPaneIDBadge(fr, 0, 0, 20, 4, 2, true, false, f)
	DrawPaneIDBadge(fr, 0, 21, 20, 12, 1, false, false, f)
	top := row(fr, 0)
	if !strings.Contains(top[:20], " #4 ") {
		t.Errorf("active pane badge = %q, want ' #4 '", top[:20])
	}
	if !strings.Contains(top[21:], " #12") {
		t.Errorf("inactive pane badge = %q, want ' #12'", top[21:])
	}
	// Verify active badge sits at the right edge of cols [0..20): " #4 " is 4 cols -> starts at x=16.
	if fr.Lines[0][16].R != ' ' || fr.Lines[0][17].R != '#' || fr.Lines[0][18].R != '4' {
		t.Errorf("active badge not right-aligned at col 16: %q", top[15:20])
	}
	if fr.Lines[0][17].Style.Bg != vt.RGB(0xf9, 0xe2, 0xaf) {
		t.Errorf("active badge style = %+v, want #f9e2af bg", fr.Lines[0][17].Style)
	}

	// Validation error on bad pane_id placeholder.
	badCfg := DefaultStatusConfig()
	badCfg.PaneID = "{unknown}"
	if _, badErr := ParseStatusConfig(badCfg); badErr == nil {
		t.Errorf("expected error for unknown placeholder in pane_id")
	}
}

func TestStatusThemeColorTags(t *testing.T) {
	cfg := DefaultStatusConfig()
	cfg.Right = "{fg=accent,bg=tab_bg,bold}X{/}{fg=theme.dim}Y{/}"
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatalf("ParseStatusConfig: %v", err)
	}

	Themes["cyberpunk"].Apply()
	fr := NewFrame(40, 1)
	DrawStatusBar(fr, 0, Status{Workspace: "main", Tabs: []string{"sh"}}, f)
	xCol, yCol := -1, -1
	for i, c := range fr.Lines[0] {
		switch c.R {
		case 'X':
			xCol = i
		case 'Y':
			yCol = i
		}
	}
	if xCol < 0 || yCol < 0 {
		t.Fatalf("missing X or Y in %q", row(fr, 0))
	}
	if fr.Lines[0][xCol].Style.Fg != Themes["cyberpunk"].Accent || fr.Lines[0][xCol].Style.Bg != Themes["cyberpunk"].TabBg {
		t.Errorf("cyberpunk X style = %+v, want accent fg & tab_bg", fr.Lines[0][xCol].Style)
	}
	if fr.Lines[0][yCol].Style.Fg != Themes["cyberpunk"].Dim {
		t.Errorf("cyberpunk Y style = %+v, want dim fg", fr.Lines[0][yCol].Style)
	}

	Themes["vice-city"].Apply()
	fr2 := NewFrame(40, 1)
	DrawStatusBar(fr2, 0, Status{Workspace: "main", Tabs: []string{"sh"}}, f)
	if fr2.Lines[0][xCol].Style.Fg != Themes["vice-city"].Accent {
		t.Errorf("vice-city X style = %+v, want vice-city accent fg", fr2.Lines[0][xCol].Style)
	}
}

func TestStatusMessageInlineColors(t *testing.T) {
	Themes["default"].Apply()
	f, err := ParseStatusConfig(DefaultStatusConfig())
	if err != nil {
		t.Fatal(err)
	}
	fr := NewFrame(60, 1)
	DrawStatusBar(fr, 0, Status{Message: "{fg=red,bold}ALERT{/} ok"}, f)
	line := row(fr, 0)
	if !strings.Contains(line, "ALERT ok") {
		t.Fatalf("status message = %q, want 'ALERT ok'", line)
	}
	col := strings.Index(line, "ALERT")
	if fr.Lines[0][col].Style.Fg != vt.Indexed(1) {
		t.Errorf("ALERT fg = %+v, want red (indexed 1)", fr.Lines[0][col].Style.Fg)
	}
}

func TestStatusZoomGlyphsPerType(t *testing.T) {
	Themes["default"].Apply()

	baseCfg := func() StatusConfig {
		cfg := DefaultStatusConfig()
		cfg.Left = "{tabs}"
		cfg.Right = "{badge} {current_pane_glyph}/{current_pane_id_glyph}"
		cfg.Tab = " {tab_glyph}{tab_index_glyph}:{pane_glyph} "
		cfg.TabGlyphs = []string{"■"}
		cfg.TabZoomGlyph = "▣"
		cfg.TabIndexGlyphs = []string{"0", "1", "2"}
		cfg.TabIndexZoomGlyphs = []string{"⓪", "①", "②"}
		cfg.PaneGlyphs = []string{"a", "b", "c"}
		cfg.PaneZoomGlyphs = []string{"A", "B", "C"}
		cfg.CurrentPaneGlyphs = []string{"x", "y", "z"}
		cfg.CurrentPaneZoomGlyphs = []string{"X", "Y", "Z"}
		cfg.CurrentPaneIDGlyphs = []string{"p", "q", "r"}
		cfg.CurrentPaneIDZoomGlyphs = []string{"P", "Q", "R"}
		return cfg
	}

	st := Status{
		Tabs:            []string{"t1"},
		TabPaneCount:    []int{2},
		TabActivePane:   []int{1},
		TabActivePaneID: []int{2},
		ActiveTab:       0,
		PaneIndex:       1,
		PaneID:          2,
		PaneCount:       2,
		Zoomed:          true,
		Badge:           "ZOOM",
	}

	render := func(cfg StatusConfig) (string, StatusFormat) {
		f, err := ParseStatusConfig(cfg)
		if err != nil {
			t.Fatal(err)
		}
		fr := NewFrame(80, 1)
		DrawStatusBar(fr, 0, st, f)
		return row(fr, 0), f
	}

	// Default: all zoom_glyphs flags are false -> normal glyphs and ZOOM badge shown.
	gotDefault, fDefault := render(baseCfg())
	if fDefault.HasZoomIndicator() {
		t.Errorf("HasZoomIndicator = true, want false by default")
	}
	if !strings.Contains(gotDefault, "■0:b") || !strings.Contains(gotDefault, "ZOOM  y/q") {
		t.Errorf("default zoom_glyphs off = %q", gotDefault)
	}

	// Each glyph type can be enabled independently.
	cases := []struct {
		name  string
		tweak func(*ZoomGlyphsConfig)
		want  []string
	}{
		{"tab", func(z *ZoomGlyphsConfig) { z.Tab = true }, []string{"▣0:b", " y/q"}},
		{"tab_index", func(z *ZoomGlyphsConfig) { z.TabIndex = true }, []string{"■⓪:b", " y/q"}},
		{"pane", func(z *ZoomGlyphsConfig) { z.Pane = true }, []string{"■0:B", " y/q"}},
		{"current_pane", func(z *ZoomGlyphsConfig) { z.CurrentPane = true }, []string{"■0:b", " Y/q"}},
		{"current_pane_id", func(z *ZoomGlyphsConfig) { z.CurrentPaneID = true }, []string{"■0:b", " y/Q"}},
	}
	for _, tc := range cases {
		cfg := baseCfg()
		tc.tweak(&cfg.ZoomGlyphs)
		got, f := render(cfg)
		if !f.HasZoomIndicator() {
			t.Errorf("%s: HasZoomIndicator = false, want true", tc.name)
		}
		if !strings.Contains(got, "ZOOM") {
			t.Errorf("%s: ZOOM badge should be visible: %q", tc.name, got)
		}
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in %q", tc.name, w, got)
			}
		}
	}
}

func TestStatusProcessGlyphs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Tab = " {tab_glyph}:{process_glyph}{name} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.ZoomGlyphs.Tab = true
	cfg.TabColors = []string{"cyan", "yellow", "green"}
	cfg.ProcessGlyphs = map[string]string{
		"ssh":  "{fg=red}S{/}",
		"nvim": "N",
	}

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	st := Status{
		Tabs:       []string{"remote", "edit", "shell"},
		TabProcess: []string{"ssh", "nvim", "bash"},
		ActiveTab:  0,
		Zoomed:     true, // ssh on zoomed active tab must still win over tab_zoom_glyph
	}

	fr := NewFrame(60, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	for _, want := range []string{"S:Sremote", "N:Nedit", "■:shell"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "▣") {
		t.Errorf("process_glyphs should take precedence over tab_zoom_glyph: %q", got)
	}
}

func TestCurrentPaneGlyphsDoNotFallbackToPaneGlyphs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = "{badge} [{current_pane_glyph}/{current_pane_id_glyph}]"
	cfg.Tab = " {pane_glyph} "
	cfg.PaneGlyphs = []string{"0", "1", "2"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷"}
	cfg.ZoomGlyphs.Pane = true

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !f.HasZoomIndicator() {
		t.Errorf("HasZoomIndicator = false, want true because {pane_glyph} is in tab")
	}

	st := Status{
		Tabs:         []string{"t1"},
		TabPaneCount: []int{2},
		ActiveTab:    0,
		PaneIndex:    1,
		PaneID:       2,
		PaneCount:    2,
		Zoomed:       true,
		Badge:        "ZOOM",
	}
	fr := NewFrame(60, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	if !strings.Contains(got, "[/]") {
		t.Errorf("current_pane_glyph/current_pane_id_glyph should not fall back to pane_glyphs: %q", got)
	}

	// Only {current_pane_glyph}/{current_pane_id_glyph} without their own arrays:
	// ZoomGlyphs.Pane must not trigger HasZoomIndicator().
	cfg2 := DefaultStatusConfig()
	cfg2.Left = "{tabs}"
	cfg2.Right = "{badge} {current_pane_glyph}/{current_pane_id_glyph}"
	cfg2.Tab = " {index} "
	cfg2.PaneGlyphs = []string{"0", "1", "2"}
	cfg2.PaneZoomGlyphs = []string{"⓿", "❶", "❷"}
	cfg2.ZoomGlyphs.Pane = true

	f2, err := ParseStatusConfig(cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if f2.HasZoomIndicator() {
		t.Errorf("HasZoomIndicator = true, want false when only current_pane_* placeholders are used without their own arrays")
	}
}

func TestStatusBadgeTheming(t *testing.T) {
	th, err := ParseTheme(map[string]string{
		"badge_fg": "#111111",
		"badge_bg": "#ffaa00",
	})
	if err != nil {
		t.Fatal(err)
	}
	th.Apply()
	defer Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Right = "{badge} "
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, badge := range []string{"MOVE", "ZOOM"} {
		fr := NewFrame(30, 1)
		DrawStatusBar(fr, 0, Status{Badge: badge, Zoomed: badge == "ZOOM"}, f)
		got := row(fr, 0)
		if !strings.Contains(got, badge) {
			t.Fatalf("missing %s badge in %q", badge, got)
		}
		foundStyled := false
		for _, c := range fr.Lines[0] {
			if c.R == rune(badge[0]) && c.Style.Fg == vt.RGB(0x11, 0x11, 0x11) && c.Style.Bg == vt.RGB(0xff, 0xaa, 0x00) {
				foundStyled = true
				break
			}
		}
		if !foundStyled {
			t.Errorf("%s badge did not use badge_fg/badge_bg", badge)
		}
	}

	// Inline style tags around {badge} override theme badge colours.
	cfgInline := DefaultStatusConfig()
	cfgInline.Right = "{fg=#eeeeee,bg=#224488}{badge}{/} "
	fInline, err := ParseStatusConfig(cfgInline)
	if err != nil {
		t.Fatal(err)
	}
	fr := NewFrame(30, 1)
	DrawStatusBar(fr, 0, Status{Badge: "MOVE"}, fInline)
	foundInline := false
	for _, c := range fr.Lines[0] {
		if c.R == 'M' && c.Style.Fg == vt.RGB(0xee, 0xee, 0xee) && c.Style.Bg == vt.RGB(0x22, 0x44, 0x88) {
			foundInline = true
			break
		}
	}
	if !foundInline {
		t.Errorf("inline {fg=...,bg=...}{badge}{/} did not apply custom badge style")
	}
}

func TestStatusInactiveZoomedTabGlyphs(t *testing.T) {
	Themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = " {tabs} "
	cfg.Right = "{badge}"
	cfg.Tab = "{tab_glyph}{tab_index_glyph}:{pane_glyph}:{current_pane_glyph}:{current_pane_id_glyph}{zoom}"
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"0", "1", "2"}
	cfg.TabIndexZoomGlyphs = []string{"⓪", "①", "②"}
	cfg.PaneGlyphs = []string{"a", "b", "c"}
	cfg.PaneZoomGlyphs = []string{"A", "B", "C"}
	cfg.CurrentPaneGlyphs = []string{"x", "y", "z"}
	cfg.CurrentPaneZoomGlyphs = []string{"X", "Y", "Z"}
	cfg.CurrentPaneIDGlyphs = []string{"p", "q", "r", "s"}
	cfg.CurrentPaneIDZoomGlyphs = []string{"P", "Q", "R", "S"}
	cfg.Zoom = "*"
	cfg.ZoomGlyphs = ZoomGlyphsConfig{
		Tab:           true,
		TabIndex:      true,
		Pane:          true,
		CurrentPane:   true,
		CurrentPaneID: true,
	}

	f, err := ParseStatusConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Tab 0 is inactive and zoomed; tab 1 is active and not zoomed.
	st := Status{
		Tabs:            []string{"one", "two"},
		TabPaneCount:    []int{2, 2},
		TabActivePane:   []int{0, 1},
		TabActivePaneID: []int{1, 3},
		TabZoomed:       []bool{true, false},
		ActiveTab:       1,
		Zoomed:          false,
	}

	fr := NewFrame(80, 1)
	DrawStatusBar(fr, 0, st, f)
	got := row(fr, 0)
	if !strings.Contains(got, "▣⓪:B:X:P*") {
		t.Errorf("inactive zoomed tab = %q, want ▣⓪:B:X:P*", got)
	}
	if !strings.Contains(got, "■1:b:y:r") || strings.Contains(got, "■1:b:y:r*") {
		t.Errorf("active unzoomed tab = %q, want ■1:b:y:r", got)
	}
}
