package ui

import (
	"io"
	"strings"
	"testing"

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

func TestStatusInlineColorsAndGlyphs(t *testing.T) {
	themes["default"].Apply()

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
	if bg := fr.Lines[0][fishCols[1]].Style.Bg; bg != themes["default"].TabActiveBg {
		t.Errorf("active tab fish bg = %v, want TabActiveBg %v", bg, themes["default"].TabActiveBg)
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
		if _, err := ParseStatusConfig(bad); err == nil {
			t.Errorf("ParseStatusConfig(%+v) should have failed", bad)
		}
	}
}

func TestStatusZoomedTabAndBadge(t *testing.T) {
	themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Tab = " {tab_glyph} {index}:{name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabColors = []string{"cyan", "yellow"}
	cfg.TabZoomGlyph = "▣"

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
	if strings.Contains(got, "ZOOM") {
		t.Errorf("ZOOM badge should be suppressed when tab_zoom_glyph is used: %q", got)
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

	// {zoom} placeholder in right side also works and suppresses ZOOM badge.
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
	if strings.Contains(got2, "ZOOM") || !strings.Contains(got2, "▣ 1/1") {
		t.Errorf("{zoom} placeholder = %q", got2)
	}
}

func TestStatusThreeGlyphTabs(t *testing.T) {
	themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
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
	if strings.Contains(got, "ZOOM") {
		t.Errorf("ZOOM badge should be suppressed when pane_zoom_glyphs is used: %q", got)
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
	themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = "{pane_glyph}"
	cfg.Tab = " {tab_glyph}{tab_index_glyph}:{pane_glyph} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}

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
	themes["default"].Apply()

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
	joined := ""
	for y := 0; y < dlg.H; y++ {
		joined += row(dlg, y) + "\n"
	}
	if !strings.Contains(joined, "rename session") || !strings.Contains(joined, "> alpha") ||
		!strings.Contains(joined, "Enter save · Esc cancel") {
		t.Errorf("dialog rendering unexpected:\n%s", joined)
	}
}

func TestDrawSplash(t *testing.T) {
	themes["default"].Apply()
	sp, err := ParseSplash([]string{"[SLAT]", " /\\ "}, "v1.0", "press ?", "cyan", "208", "#1e1e2e")
	if err != nil {
		t.Fatal(err)
	}
	fr := NewFrame(40, 10)
	DrawSplash(fr, sp)
	var out string
	for y := 0; y < fr.H; y++ {
		out += row(fr, y) + "\n"
	}
	if !strings.Contains(out, "[SLAT]") || !strings.Contains(out, "v1.0") || !strings.Contains(out, "press ?") {
		t.Errorf("splash output = %q", out)
	}
	if fr.Lines[0][0].Style.Bg != vt.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("splash bg = %v, want #1e1e2e", fr.Lines[0][0].Style.Bg)
	}
	if _, err := ParseSplash(nil, "", "", "notacolor", "", ""); err == nil {
		t.Error("expected error for invalid splash.fg")
	}
}

func TestStatusPaneGlyphCurrent(t *testing.T) {
	themes["default"].Apply()

	cfg := DefaultStatusConfig()
	cfg.Left = "{tabs}"
	cfg.Right = "{badge}{pane_glyph_current}"
	cfg.Tab = " {tab_index_glyph}:{pane_glyph_current}/{pane_glyph} "
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
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
	if !strings.HasSuffix(got, "❷") || strings.Contains(got, "ZOOM") {
		t.Errorf("right pane_glyph_current = %q", got)
	}
}

func TestStatusTabActiveAndSeparator(t *testing.T) {
	themes["default"].Apply()

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
	themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
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
	themes["default"].Apply()
	cfg := DefaultStatusConfig()
	cfg.Left = "{prefix} {workspace} │ {tabs}"
	cfg.Right = "{badge}{workspaces} {pane}/{panes} "
	cfg.Tab = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
	cfg.TabGlyphs = []string{"■"}
	cfg.TabZoomGlyph = "▣"
	cfg.TabIndexGlyphs = []string{"⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"}
	cfg.PaneGlyphs = []string{"₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"}
	cfg.PaneZoomGlyphs = []string{"⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"}
	cfg.TabColors = []string{"cyan", "magenta", "yellow"}
	cfg.PaneColors = []string{"208", "44", "#a6e3a1", "red"}
	f, err := ParseStatusConfig(cfg)
	if err != nil {
		b.Fatal(err)
	}
	f.compiled = nil
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
