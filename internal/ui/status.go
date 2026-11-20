package ui

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/vt"
)

// StatusConfig is the [status] table as read from TOML.
type StatusConfig struct {
	Left         string   `toml:"left"`
	Right        string   `toml:"right"`
	Tab          string   `toml:"tab"`
	TabActive    string   `toml:"tab_active"`
	TabSep       string   `toml:"tab_sep"`
	Alert        string   `toml:"alert"`
	Prefix       string   `toml:"prefix"`
	PrefixActive string   `toml:"prefix_active"`
	Zoom         string   `toml:"zoom"`
	TabZoomGlyph string   `toml:"tab_zoom_glyph"`
	TabColors      []string `toml:"tab_colors"`
	PaneColors     []string `toml:"pane_colors"`
	TabGlyphs      []string `toml:"tab_glyphs"`
	TabIndexGlyphs []string `toml:"tab_index_glyphs"`
	PaneGlyphs     []string `toml:"pane_glyphs"`
	PaneZoomGlyphs []string `toml:"pane_zoom_glyphs"`
}

// StatusFormat is the validated status bar layout and palettes. Each part is
// a format string of literal text, {placeholders}, and optional inline style
// tags ({fg=...,bg=...,bold}...{/}).
type StatusFormat struct {
	Left         string // from the left edge: the workspace and the tabs
	Right        string // right-aligned: the badge, pane counts, a clock
	Tab          string // repeated for each tab by {tabs}
	TabActive    string // optional override of Tab for the active tab
	TabSep       string // drawn between tabs (default " ")
	Alert        string // appended to a tab holding a pane that wants input
	Prefix       string // shown by {prefix} when the prefix key is idle
	PrefixActive string // shown by {prefix} when the prefix key is pressed
	Zoom         string // shown by {zoom} when a pane is zoomed
	TabZoomGlyph string // replaces {tab_glyph} on the active tab when zoomed
	TabColors      []vt.Color
	PaneColors     []vt.Color
	TabGlyphs      []string
	TabIndexGlyphs []string
	PaneGlyphs     []string
	PaneZoomGlyphs []string
	compiled       map[string][]compiledPart
	leftParts      []compiledPart
	rightParts     []compiledPart
	tabParts       []compiledPart
	tabActiveParts []compiledPart
	tabSepParts    []compiledPart
	alertParts     []compiledPart
	prefixParts    []compiledPart
	prefixActParts []compiledPart
	zoomParts      []compiledPart
	tabZoomParts   []compiledPart
}

// DefaultStatusFormat is the bar slat has always drawn.
var DefaultStatusFormat = StatusFormat{
	Left:         " {workspace} │ {tabs}",
	Right:        "{badge}{workspaces} pane {pane}/{panes} ",
	Tab:          " {index}:{name}{alert} ",
	TabSep:       " ",
	Alert:        " ?",
	Prefix:       "  ",
	PrefixActive: "   ",
	Zoom:         " ▣",
}

// DefaultStatusConfig returns the default [status] configuration.
func DefaultStatusConfig() StatusConfig {
	return StatusConfig{
		Left:         DefaultStatusFormat.Left,
		Right:        DefaultStatusFormat.Right,
		Tab:          DefaultStatusFormat.Tab,
		TabSep:       DefaultStatusFormat.TabSep,
		Alert:        DefaultStatusFormat.Alert,
		Prefix:       DefaultStatusFormat.Prefix,
		PrefixActive: DefaultStatusFormat.PrefixActive,
		Zoom:         DefaultStatusFormat.Zoom,
	}
}

// statusFields are the placeholders, and what each expands to. Anything
// not listed is a config error rather than a silently empty string.
var statusFields = map[string]func(Status) string{
	"session":         func(s Status) string { return s.Session },
	"workspace":       func(s Status) string { return s.Workspace },
	"workspace_index": func(s Status) string { return smallInt(s.WorkspaceIndex + 1) },
	"workspace_count": func(s Status) string { return smallInt(s.WorkspaceCount) },
	"pane":            func(s Status) string { return smallInt(s.PaneIndex + 1) },
	"panes":           func(s Status) string { return smallInt(s.PaneCount) },
	"tab":             func(s Status) string { return smallInt(s.ActiveTab + 1) },
	"tabs_count":      func(s Status) string { return smallInt(len(s.Tabs)) },
	"time":            func(s Status) string { return time.Now().Format("15:04") },
	"date":            func(s Status) string { return time.Now().Format("2006-01-02") },
	"seconds":         func(s Status) string { return time.Now().Format("15:04:05") },
	// Handled while drawing, because they bring their own styles:
	"tabs":               nil,
	"badge":              nil,
	"prefix":             nil,
	"zoom":               nil,
	"pane_glyph":         nil,
	"pane_glyph_current": nil,
	// Only when more than one workspace exists, so a single-workspace
	// session isn't cluttered by a counter that never changes.
	"workspaces": nil,
}

// StatusFieldNames lists the placeholders, for errors and docs.
func StatusFieldNames() []string {
	names := make([]string, 0, len(statusFields))
	for n := range statusFields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseStatusFormat reads a string map of [status] settings, validating
// placeholders and inline style tags.
func ParseStatusFormat(settings map[string]string) (StatusFormat, error) {
	cfg := DefaultStatusConfig()
	fields := map[string]*string{
		"left":           &cfg.Left,
		"right":          &cfg.Right,
		"tab":            &cfg.Tab,
		"tab_active":     &cfg.TabActive,
		"tab_sep":        &cfg.TabSep,
		"alert":          &cfg.Alert,
		"prefix":         &cfg.Prefix,
		"prefix_active":  &cfg.PrefixActive,
		"zoom":           &cfg.Zoom,
		"tab_zoom_glyph": &cfg.TabZoomGlyph,
	}
	for key, value := range settings {
		dst, ok := fields[key]
		if !ok {
			return DefaultStatusFormat, fmt.Errorf("unknown setting %q in [status]", key)
		}
		*dst = value
	}
	return ParseStatusConfig(cfg)
}

// ParseStatusConfig validates a StatusConfig, checking placeholders, inline
// style tags, and indexed colour lists.
func ParseStatusConfig(cfg StatusConfig) (StatusFormat, error) {
	f := StatusFormat{
		Left:           cfg.Left,
		Right:          cfg.Right,
		Tab:            cfg.Tab,
		TabActive:      cfg.TabActive,
		TabSep:         cfg.TabSep,
		Alert:          cfg.Alert,
		Prefix:         cfg.Prefix,
		PrefixActive:   cfg.PrefixActive,
		Zoom:           cfg.Zoom,
		TabZoomGlyph:   cfg.TabZoomGlyph,
		TabGlyphs:      slices.Clone(cfg.TabGlyphs),
		TabIndexGlyphs: slices.Clone(cfg.TabIndexGlyphs),
		PaneGlyphs:     slices.Clone(cfg.PaneGlyphs),
		PaneZoomGlyphs: slices.Clone(cfg.PaneZoomGlyphs),
		compiled:       make(map[string][]compiledPart, 16),
	}
	sides := StatusFieldNames()
	tabFields := []string{"alert", "index", "name", "tab_glyph", "tab_index_glyph", "pane_glyph", "pane_glyph_current", "zoom"}
	checks := []struct {
		name    string
		value   string
		allowed []string
		dst     *[]compiledPart
	}{
		{"left", f.Left, sides, &f.leftParts},
		{"right", f.Right, sides, &f.rightParts},
		{"tab", f.Tab, tabFields, &f.tabParts},
		{"tab_active", f.TabActive, tabFields, &f.tabActiveParts},
		{"tab_sep", f.TabSep, nil, &f.tabSepParts},
		{"alert", f.Alert, nil, &f.alertParts},
		{"prefix", f.Prefix, nil, &f.prefixParts},
		{"prefix_active", f.PrefixActive, nil, &f.prefixActParts},
		{"zoom", f.Zoom, nil, &f.zoomParts},
		{"tab_zoom_glyph", f.TabZoomGlyph, nil, &f.tabZoomParts},
	}
	for _, c := range checks {
		if err := checkFormat(c.value, c.allowed); err != nil {
			return f, fmt.Errorf("status.%s: %w", c.name, err)
		}
		cp := compileFormat(c.value)
		*c.dst = cp
		f.compiled[c.value] = cp
	}
	for _, list := range []struct {
		name string
		vals []string
	}{
		{"tab_glyphs", f.TabGlyphs},
		{"tab_index_glyphs", f.TabIndexGlyphs},
		{"pane_glyphs", f.PaneGlyphs},
		{"pane_zoom_glyphs", f.PaneZoomGlyphs},
	} {
		for i, g := range list.vals {
			if err := checkFormat(g, nil); err != nil {
				return f, fmt.Errorf("status.%s[%d]: %w", list.name, i, err)
			}
			f.compiled[g] = compileFormat(g)
		}
	}
	var err error
	if f.TabColors, err = parseColorList("status.tab_colors", cfg.TabColors); err != nil {
		return f, err
	}
	if f.PaneColors, err = parseColorList("status.pane_colors", cfg.PaneColors); err != nil {
		return f, err
	}
	return f, nil
}

func parseColorList(field string, raw []string) ([]vt.Color, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	colors := make([]vt.Color, len(raw))
	for i, s := range raw {
		c, err := ParseColor(s)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		colors[i] = c
	}
	return colors, nil
}

// HasPrefixPlaceholder reports whether {prefix} is used on either side of the bar.
func (f StatusFormat) HasPrefixPlaceholder() bool {
	return hasField(f.Left, "prefix") || hasField(f.Right, "prefix")
}

// HasZoomIndicator reports whether a dedicated zoom indicator is configured in
// the bar or tab format, so {badge} does not duplicate "ZOOM".
func (f StatusFormat) HasZoomIndicator() bool {
	return hasField(f.Left, "zoom") || hasField(f.Right, "zoom") ||
		hasField(f.Tab, "zoom") || hasField(f.TabActive, "zoom") ||
		(f.TabZoomGlyph != "" && (hasField(f.Tab, "tab_glyph") || hasField(f.TabActive, "tab_glyph"))) ||
		(len(f.PaneZoomGlyphs) > 0 && (hasField(f.Tab, "pane_glyph") || hasField(f.TabActive, "pane_glyph") ||
			hasField(f.Left, "pane_glyph_current") || hasField(f.Right, "pane_glyph_current") ||
			hasField(f.Tab, "pane_glyph_current") || hasField(f.TabActive, "pane_glyph_current")))
}

func hasField(format, name string) bool {
	for _, part := range splitFormat(format) {
		if part.kind == partField && part.text == name {
			return true
		}
	}
	return false
}

func (f StatusFormat) tabColor(idx int) vt.Color {
	if len(f.TabColors) == 0 || idx < 0 {
		return 0
	}
	return f.TabColors[idx%len(f.TabColors)]
}

func (f StatusFormat) paneColor(idx int) vt.Color {
	if len(f.PaneColors) == 0 || idx < 0 {
		return 0
	}
	return f.PaneColors[idx%len(f.PaneColors)]
}

func checkFormat(format string, allowed []string) error {
	depth := 0
	for _, part := range splitFormat(format) {
		switch part.kind {
		case partStylePush:
			if _, err := parseStyleTag(part.text); err != nil {
				return err
			}
			depth++
		case partStylePop:
			if depth == 0 {
				return fmt.Errorf("unmatched {/}")
			}
			depth--
		case partField:
			if slices.Contains(allowed, part.text) {
				continue
			}
			if len(allowed) == 0 {
				return fmt.Errorf("{%s}: this one takes plain text and style tags, no placeholders", part.text)
			}
			return fmt.Errorf("unknown placeholder {%s} (have %s)",
				part.text, strings.Join(allowed, ", "))
		}
	}
	return nil
}

type partKind uint8

const (
	partLiteral partKind = iota
	partField
	partStylePush
	partStylePop
)

type formatPart struct {
	text string
	kind partKind
}

// splitFormat breaks a format string into literal runs, {placeholders}, and
// inline style tags ({fg=...,bg=...,bold}...{/}). An unterminated { is
// literal text, which is friendlier than an error for someone whose bar
// contains a stray brace.
func splitFormat(format string) []formatPart {
	var parts []formatPart
	for len(format) > 0 {
		open := strings.IndexByte(format, '{')
		if open < 0 {
			return append(parts, formatPart{text: format, kind: partLiteral})
		}
		close := strings.IndexByte(format[open:], '}')
		if close < 0 {
			return append(parts, formatPart{text: format, kind: partLiteral})
		}
		if open > 0 {
			parts = append(parts, formatPart{text: format[:open], kind: partLiteral})
		}
		inner := format[open+1 : open+close]
		kind := partField
		switch {
		case inner == "/":
			kind = partStylePop
		case isStyleTag(inner):
			kind = partStylePush
		}
		parts = append(parts, formatPart{text: inner, kind: kind})
		format = format[open+close+1:]
	}
	return parts
}

func isStyleTag(inner string) bool {
	if strings.ContainsRune(inner, '=') {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(inner)) {
	case "bold", "dim", "italic", "underline", "reverse":
		return true
	}
	return false
}

type colorRef struct {
	set     bool
	color   vt.Color
	dynamic string // "", "tab", "pane", "default"
}

type styleSpec struct {
	fg    colorRef
	bg    colorRef
	attrs vt.Attr
}

func parseStyleTag(inner string) (styleSpec, error) {
	var spec styleSpec
	for _, raw := range strings.Split(inner, ",") {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			return spec, fmt.Errorf("{%s}: empty style attribute", inner)
		}
		if k, v, ok := strings.Cut(tok, "="); ok {
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.ToLower(strings.TrimSpace(v))
			var ref colorRef
			switch v {
			case "tab", "pane", "default":
				ref = colorRef{set: true, dynamic: v}
			default:
				c, err := ParseColor(v)
				if err != nil {
					return spec, fmt.Errorf("{%s}: %w", inner, err)
				}
				ref = colorRef{set: true, color: c}
			}
			switch k {
			case "fg":
				spec.fg = ref
			case "bg":
				spec.bg = ref
			default:
				return spec, fmt.Errorf("{%s}: unknown style key %q", inner, k)
			}
			continue
		}
		switch strings.ToLower(tok) {
		case "bold":
			spec.attrs |= vt.Bold
		case "dim":
			spec.attrs |= vt.Faint
		case "italic":
			spec.attrs |= vt.Italic
		case "underline":
			spec.attrs |= vt.Underline
		case "reverse":
			spec.attrs |= vt.Reverse
		default:
			return spec, fmt.Errorf("{%s}: unknown style attribute %q", inner, tok)
		}
	}
	return spec, nil
}

type styleCtx struct {
	tabColor  vt.Color
	paneColor vt.Color
}

func resolveStyle(base vt.Style, spec styleSpec, ctx styleCtx) vt.Style {
	out := base
	out.Attrs |= spec.attrs
	if spec.fg.set {
		out.Fg = resolveColorRef(spec.fg, ctx)
	}
	if spec.bg.set {
		out.Bg = resolveColorRef(spec.bg, ctx)
	}
	return out
}

func resolveColorRef(ref colorRef, ctx styleCtx) vt.Color {
	switch ref.dynamic {
	case "tab":
		if ctx.tabColor != 0 {
			return ctx.tabColor
		}
		return styleAccent
	case "pane":
		if ctx.paneColor != 0 {
			return ctx.paneColor
		}
		return styleAccent
	case "default":
		return vt.DefaultColor
	default:
		return ref.color
	}
}

type compiledPart struct {
	kind partKind
	text string
	spec styleSpec
}

func compileFormat(format string) []compiledPart {
	raw := splitFormat(format)
	out := make([]compiledPart, len(raw))
	for i, p := range raw {
		out[i] = compiledPart{kind: p.kind, text: p.text}
		if p.kind == partStylePush {
			if spec, err := parseStyleTag(p.text); err == nil {
				out[i].spec = spec
			}
		}
	}
	return out
}

func (f StatusFormat) parts(format string) []compiledPart {
	if p, ok := f.compiled[format]; ok {
		return p
	}
	return compileFormat(format)
}

var smallInts = func() [100]string {
	var a [100]string
	for i := range a {
		a[i] = strconv.Itoa(i)
	}
	return a
}()

func smallInt(n int) string {
	if n >= 0 && n < len(smallInts) {
		return smallInts[n]
	}
	return strconv.Itoa(n)
}

type barSink struct {
	frame  *Frame // nil when measuring width only
	x, y   int
	budget int
}

func (b *barSink) emit(text string, st vt.Style) {
	if text == "" {
		return
	}
	if b.frame == nil {
		b.x += vt.StringWidth(text)
		return
	}
	if b.x >= b.budget {
		return
	}
	if b.x+vt.StringWidth(text) > b.budget {
		text = Truncate(text, b.budget-b.x)
	}
	b.x = Text(b.frame, b.x, b.y, text, st)
}

func emitPlainParts(b *barSink, parts []compiledPart, base vt.Style, ctx styleCtx) {
	var stackBuf [8]vt.Style
	stackBuf[0] = base
	stack := stackBuf[:1]
	for _, part := range parts {
		cur := stack[len(stack)-1]
		switch part.kind {
		case partLiteral:
			b.emit(part.text, cur)
		case partStylePush:
			stack = append(stack, resolveStyle(cur, part.spec, ctx))
		case partStylePop:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func emitPlain(b *barSink, f StatusFormat, format string, base vt.Style, ctx styleCtx) {
	emitPlainParts(b, f.parts(format), base, ctx)
}

func emitNumberGlyphs(b *barSink, f StatusFormat, glyphs []string, n int, st vt.Style, ctx styleCtx) {
	if len(glyphs) == 0 || n <= 0 {
		return
	}
	if len(glyphs) != 10 {
		emitPlain(b, f, glyphs[(n-1)%len(glyphs)], st, ctx)
		return
	}
	var digits [20]int
	count := 0
	for v := n; v > 0; v /= 10 {
		digits[count] = v % 10
		count++
	}
	for j := count - 1; j >= 0; j-- {
		emitPlain(b, f, glyphs[digits[j]], st, ctx)
	}
}

// emitStatus lays out one side of the bar directly into b (measuring width
// when b.frame == nil, or drawing onto b.frame otherwise).
func emitStatus(b *barSink, format string, st Status, f StatusFormat) {
	ctx := styleCtx{
		tabColor:  f.tabColor(st.ActiveTab),
		paneColor: f.paneColor(st.PaneIndex),
	}
	sideParts := f.parts(format)
	switch format {
	case f.Left:
		if f.leftParts != nil {
			sideParts = f.leftParts
		}
	case f.Right:
		if f.rightParts != nil {
			sideParts = f.rightParts
		}
	}
	var stackBuf [8]vt.Style
	stackBuf[0] = styleBar
	stack := stackBuf[:1]
	for _, part := range sideParts {
		if b.frame != nil && b.x >= b.budget {
			return
		}
		cur := stack[len(stack)-1]
		switch part.kind {
		case partLiteral:
			b.emit(part.text, cur)
		case partStylePush:
			stack = append(stack, resolveStyle(cur, part.spec, ctx))
		case partStylePop:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case partField:
			switch part.text {
			case "tabs":
				emitTabs(b, st, f, ctx)
			case "badge":
				if st.Badge != "" {
					b.emit(" ", styleBadge)
					b.emit(st.Badge, styleBadge)
					b.emit(" ", styleBadge)
				}
			case "prefix":
				pFormat, pStyle := f.Prefix, stylePrefix
				if st.PrefixActive {
					pFormat, pStyle = f.PrefixActive, stylePrefixActive
				}
				emitPlain(b, f, pFormat, pStyle, ctx)
			case "zoom":
				if st.Zoomed {
					emitPlain(b, f, f.Zoom, cur, ctx)
				}
			case "pane_glyph":
				if len(f.PaneGlyphs) == 0 || st.PaneIndex < 0 {
					continue
				}
				s := cur
				if c := f.paneColor(st.PaneIndex); c != 0 && cur == styleBar {
					s.Fg = c
				}
				emitNumberGlyphs(b, f, f.PaneGlyphs, st.PaneIndex+1, s, ctx)
			case "pane_glyph_current":
				if st.PaneIndex < 0 {
					continue
				}
				glyphs := f.PaneGlyphs
				if st.Zoomed && len(f.PaneZoomGlyphs) > 0 {
					glyphs = f.PaneZoomGlyphs
				}
				if len(glyphs) == 0 {
					continue
				}
				s := cur
				if c := f.paneColor(st.PaneIndex); c != 0 && cur == styleBar {
					s.Fg = c
				}
				emitNumberGlyphs(b, f, glyphs, st.PaneIndex+1, s, ctx)
			case "workspaces":
				if st.WorkspaceCount <= 1 {
					continue
				}
				s := styleDim
				if cur != styleBar {
					s = cur
				}
				b.emit(" ws ", s)
				b.emit(smallInt(st.WorkspaceIndex+1), s)
				b.emit("/", s)
				b.emit(smallInt(st.WorkspaceCount), s)
				b.emit(" ·", s)
			case "session":
				s := styleWorkspace
				if cur != styleBar {
					s = cur
				}
				b.emit(st.Session, s)
			case "workspace":
				s := styleWorkspace
				if cur != styleBar {
					s = cur
				}
				b.emit(st.Workspace, s)
			default:
				if expand := statusFields[part.text]; expand != nil {
					s := styleDim
					if cur != styleBar {
						s = cur
					}
					b.emit(expand(st), s)
				}
			}
		}
	}
}

func emitTabs(b *barSink, st Status, f StatusFormat, ctx styleCtx) {
	for i, name := range st.Tabs {
		if b.frame != nil && b.x >= b.budget {
			return
		}
		tabBase := styleTab
		if i == st.ActiveTab {
			tabBase = styleTabActive
		}
		tabCtx := styleCtx{tabColor: f.tabColor(i), paneColor: ctx.paneColor}
		hasAlert := i < len(st.TabAlert) && st.TabAlert[i]
		paneCount := 1
		if i < len(st.TabPaneCount) && st.TabPaneCount[i] > 0 {
			paneCount = st.TabPaneCount[i]
		}
		tabParts := f.tabParts
		if i == st.ActiveTab && f.TabActive != "" {
			tabParts = f.tabActiveParts
		}
		if tabParts == nil {
			tabFmt := f.Tab
			if i == st.ActiveTab && f.TabActive != "" {
				tabFmt = f.TabActive
			}
			tabParts = f.parts(tabFmt)
		}
		var stackBuf [8]vt.Style
		stackBuf[0] = tabBase
		stack := stackBuf[:1]
		for _, part := range tabParts {
			if b.frame != nil && b.x >= b.budget {
				return
			}
			cur := stack[len(stack)-1]
			switch part.kind {
			case partLiteral:
				b.emit(part.text, cur)
			case partStylePush:
				stack = append(stack, resolveStyle(cur, part.spec, tabCtx))
			case partStylePop:
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			case partField:
				switch part.text {
				case "index":
					b.emit(smallInt(i+1), cur)
				case "name":
					b.emit(name, cur)
				case "tab_glyph":
					s := cur
					if c := f.tabColor(i); c != 0 && cur == tabBase {
						s.Fg = c
					}
					if i == st.ActiveTab && st.Zoomed && f.TabZoomGlyph != "" {
						emitPlain(b, f, f.TabZoomGlyph, s, tabCtx)
					} else if len(f.TabGlyphs) > 0 {
						emitPlain(b, f, f.TabGlyphs[i%len(f.TabGlyphs)], s, tabCtx)
					}
				case "tab_index_glyph":
					if len(f.TabIndexGlyphs) > 0 {
						s := cur
						if c := f.tabColor(i); c != 0 && cur == tabBase {
							s.Fg = c
						}
						emitNumberGlyphs(b, f, f.TabIndexGlyphs, i+1, s, tabCtx)
					}
				case "pane_glyph":
					glyphs := f.PaneGlyphs
					if i == st.ActiveTab && st.Zoomed && len(f.PaneZoomGlyphs) > 0 {
						glyphs = f.PaneZoomGlyphs
					}
					if len(glyphs) > 0 {
						s := cur
						if c := f.paneColor(paneCount - 1); c != 0 && cur == tabBase {
							s.Fg = c
						}
						pCtx := styleCtx{tabColor: f.tabColor(i), paneColor: f.paneColor(paneCount - 1)}
						emitNumberGlyphs(b, f, glyphs, paneCount, s, pCtx)
					}
				case "pane_glyph_current":
					activeIdx := 0
					if i < len(st.TabActivePane) && st.TabActivePane[i] >= 0 {
						activeIdx = st.TabActivePane[i]
					}
					glyphs := f.PaneGlyphs
					if i == st.ActiveTab && st.Zoomed && len(f.PaneZoomGlyphs) > 0 {
						glyphs = f.PaneZoomGlyphs
					}
					if len(glyphs) > 0 {
						s := cur
						if c := f.paneColor(activeIdx); c != 0 && cur == tabBase {
							s.Fg = c
						}
						pCtx := styleCtx{tabColor: f.tabColor(i), paneColor: f.paneColor(activeIdx)}
						emitNumberGlyphs(b, f, glyphs, activeIdx+1, s, pCtx)
					}
				case "zoom":
					if i == st.ActiveTab && st.Zoomed {
						emitPlain(b, f, f.Zoom, cur, tabCtx)
					}
				case "alert":
					if hasAlert {
						emitPlain(b, f, f.Alert, cur, tabCtx)
					}
				}
			}
		}
		if i < len(st.Tabs)-1 && f.TabSep != "" {
			emitPlain(b, f, f.TabSep, styleBar, ctx)
		}
	}
}
