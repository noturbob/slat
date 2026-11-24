package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/noturbob/slat/internal/vt"
)

//go:embed themes/*.toml
var embeddedThemesFS embed.FS

// Theme is the colour of everything slat draws itself: the status bar, the
// pane borders and the overlays. Pane contents are never restyled — that's
// the program's business.
type Theme struct {
	Bg             vt.Color // overlay background (and status bar default)
	Fg             vt.Color // status bar text
	Dim            vt.Color // secondary text: pane count, workspace count
	Accent         vt.Color // workspace name, focused border, badges, banner
	Border         vt.Color // borders between unfocused panes
	StatusBg       vt.Color // optional status bar background override (0 = transparent when HasStatusBg)
	TabBg          vt.Color
	TabFg          vt.Color
	TabActiveBg    vt.Color
	TabActiveFg    vt.Color
	PrefixFg       vt.Color // status {prefix} text when idle
	PrefixBg       vt.Color // status {prefix} background when idle
	PrefixActiveFg vt.Color // status {prefix} text when prefix key is pressed
	PrefixActiveBg vt.Color // status {prefix} background when prefix key is pressed
	BadgeFg        vt.Color // status {badge} text (MOVE, ZOOM, PREFIX, ...)
	BadgeBg        vt.Color // status {badge} background

	HasStatusBg       bool
	HasTabBg          bool
	HasTabActiveBg    bool
	HasPrefixBg       bool
	HasPrefixActiveBg bool
	HasBadgeBg        bool

	Vars map[string]vt.Color // custom palette variables ([vars] / [theme.vars])
}

type themeFile struct {
	Bg             string            `toml:"bg"`
	Background     string            `toml:"background"`
	Fg             string            `toml:"fg"`
	Foreground     string            `toml:"foreground"`
	Dim            string            `toml:"dim"`
	Accent         string            `toml:"accent"`
	Border         string            `toml:"border"`
	StatusBg       string            `toml:"status_bg"`
	TabBg          string            `toml:"tab_bg"`
	TabFg          string            `toml:"tab_fg"`
	TabActiveBg    string            `toml:"tab_active_bg"`
	TabActiveFg    string            `toml:"tab_active_fg"`
	PrefixFg       string            `toml:"prefix_fg"`
	PrefixBg       string            `toml:"prefix_bg"`
	PrefixActiveFg string            `toml:"prefix_active_fg"`
	PrefixActiveBg string            `toml:"prefix_active_bg"`
	BadgeFg        string            `toml:"badge_fg"`
	BadgeBg        string            `toml:"badge_bg"`
	Vars           map[string]string `toml:"vars"`
}

func (tf themeFile) toSettings() (map[string]string, error) {
	if tf.Bg != "" && tf.Background != "" {
		return nil, fmt.Errorf("cannot set both \"bg\" and \"background\"")
	}
	if tf.Fg != "" && tf.Foreground != "" {
		return nil, fmt.Errorf("cannot set both \"fg\" and \"foreground\"")
	}
	s := make(map[string]string)
	add := func(k, v string) {
		if v != "" {
			s[k] = v
		}
	}
	add("bg", tf.Bg)
	add("background", tf.Background)
	add("fg", tf.Fg)
	add("foreground", tf.Foreground)
	add("dim", tf.Dim)
	add("accent", tf.Accent)
	add("border", tf.Border)
	add("status_bg", tf.StatusBg)
	add("tab_bg", tf.TabBg)
	add("tab_fg", tf.TabFg)
	add("tab_active_bg", tf.TabActiveBg)
	add("tab_active_fg", tf.TabActiveFg)
	add("prefix_fg", tf.PrefixFg)
	add("prefix_bg", tf.PrefixBg)
	add("prefix_active_fg", tf.PrefixActiveFg)
	add("prefix_active_bg", tf.PrefixActiveBg)
	add("badge_fg", tf.BadgeFg)
	add("badge_bg", tf.BadgeBg)
	return s, nil
}

// themes are the built-in palettes loaded from embedded themes/*.toml files.
var themes = loadEmbeddedThemes()

func loadEmbeddedThemes() map[string]Theme {
	entries, err := fs.ReadDir(embeddedThemesFS, "themes")
	if err != nil {
		panic(fmt.Sprintf("embedded themes: %v", err))
	}
	raw := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, err := fs.ReadFile(embeddedThemesFS, "themes/"+e.Name())
		if err != nil {
			panic(fmt.Sprintf("embedded theme %s: %v", e.Name(), err))
		}
		name := strings.ToLower(strings.TrimSuffix(e.Name(), ".toml"))
		raw[name] = data
	}
	defData, ok := raw["default"]
	if !ok {
		panic("embedded themes: missing default.toml")
	}
	defTheme, err := parseThemeTOML(defData, Theme{})
	if err != nil {
		panic(fmt.Sprintf("embedded theme default.toml: %v", err))
	}
	out := map[string]Theme{"default": defTheme}
	for name, data := range raw {
		if name == "default" {
			continue
		}
		th, err := parseThemeTOML(data, defTheme)
		if err != nil {
			panic(fmt.Sprintf("embedded theme %s.toml: %v", name, err))
		}
		out[name] = th
	}
	return out
}

func parseThemeTOML(data []byte, base Theme) (Theme, error) {
	var tf themeFile
	md, err := toml.Decode(string(data), &tf)
	if err != nil {
		return base, err
	}
	if len(md.Undecoded()) > 0 {
		return base, fmt.Errorf("unknown setting %q", md.Undecoded()[0].String())
	}
	settings, err := tf.toSettings()
	if err != nil {
		return base, err
	}
	// Preserve explicit empty strings for transparent *_bg keys when defined in TOML.
	for _, key := range []string{"status_bg", "tab_bg", "tab_active_bg", "prefix_bg", "prefix_active_bg"} {
		if md.IsDefined(key) {
			if _, ok := settings[key]; !ok {
				settings[key] = ""
			}
		}
	}
	return applyThemeOverrides(base, settings, tf.Vars)
}

// LoadThemesFromDir loads user-defined *.toml themes from dir.
// A missing directory is not an error and returns (nil, nil).
func LoadThemesFromDir(dir string) (map[string]Theme, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make(map[string]Theme)
	base := themes["default"]
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".toml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		th, err := parseThemeTOML(data, base)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		name := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		out[name] = th
	}
	return out, nil
}

// ThemeNames lists the built-in palettes, for error messages and docs.
func ThemeNames() []string {
	return ThemeNamesFrom(nil)
}

// ThemeNamesFrom lists built-in palettes merged with any extra user themes.
func ThemeNamesFrom(extra map[string]Theme) []string {
	seen := make(map[string]struct{}, len(themes)+len(extra))
	for n := range themes {
		seen[n] = struct{}{}
	}
	for n := range extra {
		seen[n] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ThemeInfo describes an available theme for CLI listing.
type ThemeInfo struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Active bool   `json:"active"`
}

// ListThemes returns all built-in and user themes sorted by name.
func ListThemes(userThemes map[string]Theme, activeName string) []ThemeInfo {
	active := strings.ToLower(strings.TrimSpace(activeName))
	if active == "" {
		active = "default"
	}
	names := ThemeNamesFrom(userThemes)
	out := make([]ThemeInfo, 0, len(names))
	for _, n := range names {
		_, isBuiltIn := themes[n]
		_, isUser := userThemes[n]
		source := "built-in"
		if isBuiltIn && isUser {
			source = "user (overrides built-in)"
		} else if isUser {
			source = "user"
		}
		out = append(out, ThemeInfo{
			Name:   n,
			Source: source,
			Active: n == active,
		})
	}
	return out
}

func formatColorValue(c vt.Color) string {
	if c == vt.DefaultColor {
		return "transparent"
	}
	if c>>24 == 2 {
		v := uint32(c)
		return fmt.Sprintf("#%02x%02x%02x", (v>>16)&0xff, (v>>8)&0xff, v&0xff)
	}
	return strconv.Itoa(int(c & 0xff))
}

// FormatThemeTOML serializes t into a standalone theme TOML file.
func FormatThemeTOML(t Theme) []byte {
	var b strings.Builder
	writeKV := func(k string, c vt.Color) {
		fmt.Fprintf(&b, "%-16s = %q\n", k, formatColorValue(c))
	}
	writeKV("bg", t.Bg)
	writeKV("fg", t.Fg)
	writeKV("dim", t.Dim)
	writeKV("accent", t.Accent)
	writeKV("border", t.Border)
	if t.HasStatusBg {
		writeKV("status_bg", t.StatusBg)
	}
	writeKV("tab_bg", t.TabBg)
	writeKV("tab_fg", t.TabFg)
	writeKV("tab_active_bg", t.TabActiveBg)
	writeKV("tab_active_fg", t.TabActiveFg)
	if t.PrefixFg != 0 {
		writeKV("prefix_fg", t.PrefixFg)
	}
	if t.HasPrefixBg {
		writeKV("prefix_bg", t.PrefixBg)
	}
	if t.PrefixActiveFg != 0 {
		writeKV("prefix_active_fg", t.PrefixActiveFg)
	}
	if t.HasPrefixActiveBg {
		writeKV("prefix_active_bg", t.PrefixActiveBg)
	}
	if t.BadgeFg != 0 {
		writeKV("badge_fg", t.BadgeFg)
	}
	if t.HasBadgeBg {
		writeKV("badge_bg", t.BadgeBg)
	}
	if len(t.Vars) > 0 {
		b.WriteString("\n[vars]\n")
		keys := make([]string, 0, len(t.Vars))
		for k := range t.Vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%-16s = %q\n", k, formatColorValue(t.Vars[k]))
		}
	}
	return []byte(b.String())
}

// ExportThemeTOML returns the TOML bytes and canonical name for a named theme
// (or for customOverride when exporting the active customized theme).
func ExportThemeTOML(name string, userThemes map[string]Theme, customOverride *Theme) ([]byte, string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = "default"
	}
	if customOverride != nil {
		return FormatThemeTOML(*customOverride), key, nil
	}
	if th, ok := userThemes[key]; ok {
		return FormatThemeTOML(th), key, nil
	}
	if _, ok := themes[key]; ok {
		data, err := fs.ReadFile(embeddedThemesFS, "themes/"+key+".toml")
		if err != nil {
			return nil, "", err
		}
		return data, key, nil
	}
	return nil, "", fmt.Errorf("unknown theme %q (have %s)", name, strings.Join(ThemeNamesFrom(userThemes), ", "))
}

// ParseTheme builds a theme from the [theme] table: a "name" picking a
// built-in palette, plus any single colour overriding it. Anything it
// can't honour exactly is an error, so a typo in a config is reported
// rather than silently ignored.
func ParseTheme(settings map[string]string) (Theme, error) {
	return ParseThemeWith(settings, nil, nil)
}

// ParseThemeWith builds a theme from [theme] settings, optional [theme.vars],
// and optional user-defined themes (which take precedence over built-ins).
func ParseThemeWith(settings, vars map[string]string, userThemes map[string]Theme) (Theme, error) {
	t := themes["default"]
	if name, ok := settings["name"]; ok {
		key := strings.ToLower(strings.TrimSpace(name))
		if preset, known := userThemes[key]; known {
			t = preset
		} else if preset, known := themes[key]; known {
			t = preset
		} else {
			return t, fmt.Errorf("theme.name = %q: no such theme (have %s)",
				name, strings.Join(ThemeNamesFrom(userThemes), ", "))
		}
	}
	return applyThemeOverrides(t, settings, vars)
}

func normalizeThemeKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "background":
		return "bg"
	case "foreground":
		return "fg"
	default:
		return strings.ToLower(strings.TrimSpace(key))
	}
}

func applyThemeOverrides(base Theme, settings, vars map[string]string) (Theme, error) {
	t := base
	if len(base.Vars) > 0 {
		t.Vars = maps.Clone(base.Vars)
	}

	normSettings := make(map[string]string, len(settings))
	for rawKey, value := range settings {
		if rawKey == "name" {
			continue
		}
		key := normalizeThemeKey(rawKey)
		if _, exists := normSettings[key]; exists {
			return t, fmt.Errorf("duplicate setting %q in [theme]", rawKey)
		}
		normSettings[key] = value
	}

	fields := map[string]*vt.Color{
		"bg": &t.Bg, "fg": &t.Fg, "dim": &t.Dim, "accent": &t.Accent,
		"border": &t.Border, "status_bg": &t.StatusBg,
		"tab_bg": &t.TabBg, "tab_fg": &t.TabFg,
		"tab_active_bg": &t.TabActiveBg, "tab_active_fg": &t.TabActiveFg,
		"prefix_fg": &t.PrefixFg, "prefix_bg": &t.PrefixBg,
		"prefix_active_fg": &t.PrefixActiveFg, "prefix_active_bg": &t.PrefixActiveBg,
		"badge_fg": &t.BadgeFg, "badge_bg": &t.BadgeBg,
	}
	bgFlags := map[string]*bool{
		"status_bg":        &t.HasStatusBg,
		"tab_bg":           &t.HasTabBg,
		"tab_active_bg":    &t.HasTabActiveBg,
		"prefix_bg":        &t.HasPrefixBg,
		"prefix_active_bg": &t.HasPrefixActiveBg,
		"badge_bg":         &t.HasBadgeBg,
	}
	for key := range normSettings {
		if _, ok := fields[key]; !ok {
			return t, fmt.Errorf("unknown setting %q in [theme]", key)
		}
	}

	normVars := make(map[string]string, len(vars))
	for k, v := range vars {
		nk := strings.ToLower(strings.TrimSpace(k))
		if nk == "" {
			return t, fmt.Errorf("empty variable name in [theme.vars]")
		}
		normVars[nk] = v
	}
	if len(normVars) > 0 && t.Vars == nil {
		t.Vars = make(map[string]vt.Color, len(normVars))
	}

	resolvedFields := make(map[string]vt.Color, len(normSettings))
	resolvedVars := make(map[string]vt.Color, len(normVars))
	visiting := make(map[string]bool)

	var resolveEntry func(kind, key string) (vt.Color, error)
	resolveEntry = func(kind, key string) (vt.Color, error) {
		visitKey := kind + ":" + key
		if visiting[visitKey] {
			return 0, fmt.Errorf("cycle in theme variable reference %q", key)
		}
		visiting[visitKey] = true
		defer delete(visiting, visitKey)

		if kind == "var" {
			if c, ok := resolvedVars[key]; ok {
				return c, nil
			}
			rawVal, ok := normVars[key]
			if !ok {
				if c, ok := t.Vars[key]; ok {
					return c, nil
				}
				return 0, fmt.Errorf("unknown theme variable %q", "theme."+key)
			}
			c, err := resolveValue(rawVal, false)
			if err != nil {
				return 0, fmt.Errorf("theme.vars.%s: %w", key, err)
			}
			resolvedVars[key] = c
			t.Vars[key] = c
			return c, nil
		}

		// kind == "field"
		key = normalizeThemeKey(key)
		if c, ok := resolvedFields[key]; ok {
			return c, nil
		}
		rawVal, ok := normSettings[key]
		if !ok {
			if c, ok := t.ResolveColor(key); ok {
				return c, nil
			}
			return 0, fmt.Errorf("unknown theme variable %q", "theme."+key)
		}
		flag, allowTransparent := bgFlags[key]
		if allowTransparent && isTransparent(rawVal) {
			*flag = true
			resolvedFields[key] = vt.DefaultColor
			*fields[key] = vt.DefaultColor
			return vt.DefaultColor, nil
		}
		c, err := resolveValue(rawVal, allowTransparent)
		if err != nil {
			return 0, fmt.Errorf("theme.%s: %w", key, err)
		}
		if allowTransparent {
			*flag = true
		}
		resolvedFields[key] = c
		*fields[key] = c
		return c, nil
	}

	resolveValue = func(raw string, _ bool) (vt.Color, error) {
		trimmed := strings.ToLower(strings.TrimSpace(raw))
		if ref, ok := strings.CutPrefix(trimmed, "theme."); ok {
			normRef := normalizeThemeKey(ref)
			if _, isField := fields[normRef]; isField {
				return resolveEntry("field", normRef)
			}
			return resolveEntry("var", normRef)
		}
		return ParseColor(raw)
	}

	for key := range normVars {
		if _, err := resolveEntry("var", key); err != nil {
			return t, err
		}
	}
	for key := range normSettings {
		if _, err := resolveEntry("field", key); err != nil {
			return t, err
		}
	}
	return t, nil
}

var resolveValue func(raw string, allowTransparent bool) (vt.Color, error)

func isTransparent(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "transparent", "none", "default":
		return true
	}
	return false
}

var activeTheme = themes["default"]

// ResolveColor resolves a normalized [theme] colour key (including
// background/foreground aliases and custom Vars) against t.
func (t Theme) ResolveColor(key string) (vt.Color, bool) {
	switch normalizeThemeKey(key) {
	case "bg":
		return t.Bg, true
	case "fg":
		return t.Fg, true
	case "dim":
		return t.Dim, true
	case "accent":
		return t.Accent, true
	case "border":
		return t.Border, true
	case "status_bg":
		if t.HasStatusBg {
			return t.StatusBg, true
		}
		return t.Bg, true
	case "tab_bg":
		return t.TabBg, true
	case "tab_fg":
		return t.TabFg, true
	case "tab_active_bg":
		return t.TabActiveBg, true
	case "tab_active_fg":
		return t.TabActiveFg, true
	case "prefix_fg":
		return fallbackColor(t.PrefixFg, t.Fg), true
	case "prefix_bg":
		if t.HasPrefixBg {
			return t.PrefixBg, true
		}
		barBg := t.Bg
		if t.HasStatusBg {
			barBg = t.StatusBg
		}
		return fallbackColor(t.PrefixBg, barBg), true
	case "prefix_active_fg":
		return fallbackColor(t.PrefixActiveFg, t.Bg), true
	case "prefix_active_bg":
		if t.HasPrefixActiveBg {
			return t.PrefixActiveBg, true
		}
		return fallbackColor(t.PrefixActiveBg, t.Accent), true
	case "badge_fg":
		return fallbackColor(t.BadgeFg, t.Bg), true
	case "badge_bg":
		if t.HasBadgeBg {
			return t.BadgeBg, true
		}
		return fallbackColor(t.BadgeBg, t.Accent), true
	default:
		if c, ok := t.Vars[strings.ToLower(strings.TrimSpace(key))]; ok {
			return c, true
		}
		return 0, false
	}
}

// ParseColorInTheme parses a colour string, resolving "theme.<key>" references
// against t before falling back to ParseColor.
func ParseColorInTheme(s string, t Theme) (vt.Color, error) {
	norm := strings.ToLower(strings.TrimSpace(s))
	if ref, ok := strings.CutPrefix(norm, "theme."); ok {
		c, found := t.ResolveColor(ref)
		if !found {
			return 0, fmt.Errorf("unknown theme variable %q", s)
		}
		return c, nil
	}
	return ParseColor(s)
}

// themeColor resolves a normalized [theme] colour key from the currently applied theme.
func themeColor(key string) (vt.Color, bool) {
	return activeTheme.ResolveColor(key)
}

// Apply makes t the theme everything drawn from now on uses.
func (t Theme) Apply() {
	activeTheme = t
	barBg := t.Bg
	if t.HasStatusBg {
		barBg = t.StatusBg
	}
	prefixBg := fallbackColor(t.PrefixBg, barBg)
	if t.HasPrefixBg {
		prefixBg = t.PrefixBg
	}
	prefixActiveBg := fallbackColor(t.PrefixActiveBg, t.Accent)
	if t.HasPrefixActiveBg {
		prefixActiveBg = t.PrefixActiveBg
	}
	badgeBg := fallbackColor(t.BadgeBg, t.Accent)
	if t.HasBadgeBg {
		badgeBg = t.BadgeBg
	}
	styleBar = vt.Style{Fg: t.Fg, Bg: barBg}
	styleWorkspace = vt.Style{Fg: t.Accent, Bg: barBg, Attrs: vt.Bold}
	styleTab = vt.Style{Fg: t.TabFg, Bg: t.TabBg}
	styleTabActive = vt.Style{Fg: t.TabActiveFg, Bg: t.TabActiveBg, Attrs: vt.Bold}
	styleBadge = vt.Style{
		Fg:    fallbackColor(t.BadgeFg, t.Bg),
		Bg:    badgeBg,
		Attrs: vt.Bold,
	}
	styleDim = vt.Style{Fg: t.Dim, Bg: barBg}
	styleAccent = t.Accent
	stylePrefix = vt.Style{
		Fg: fallbackColor(t.PrefixFg, t.Fg),
		Bg: prefixBg,
	}
	stylePrefixActive = vt.Style{
		Fg:    fallbackColor(t.PrefixActiveFg, t.Bg),
		Bg:    prefixActiveBg,
		Attrs: vt.Bold,
	}
	styleBorder = vt.Style{Fg: t.Border}
	styleBorderOn = vt.Style{Fg: t.Accent, Attrs: vt.Bold}
	styleBox = vt.Style{Fg: t.Accent, Bg: t.Bg}
	styleBoxKey = vt.Style{Fg: t.Fg, Bg: t.Bg, Attrs: vt.Bold}
	styleBoxText = vt.Style{Fg: t.Fg, Bg: t.Bg}
	styleBoxHead = vt.Style{Fg: t.Accent, Bg: t.Bg, Attrs: vt.Bold}
	styleFinder = styleBox
	styleFinderKey = styleBoxKey
	styleFinderText = styleBoxText
	styleFinderHead = styleBoxHead
	styleFinderSel = styleTabActive
	styleStack = styleTab
	styleStackActive = styleTabActive
	styleBanner = vt.Style{Fg: t.Accent, Attrs: vt.Bold}
	styleBannerSub = vt.Style{Fg: t.Dim}
}

func fallbackColor(c, def vt.Color) vt.Color {
	if c != 0 {
		return c
	}
	return def
}

// DialogTheme holds optional colour overrides for modal dialogs and overlays.
type DialogTheme struct {
	Fg       string `toml:"fg"`
	SubFg    string `toml:"sub_fg"`
	HeadFg   string `toml:"head_fg"`
	BorderFg string `toml:"border_fg"`
	Bg       string `toml:"bg"`
}

// DialogStyle is a validated set of dialog colour overrides.
type DialogStyle struct {
	Fg, SubFg, HeadFg, BorderFg, Bg          vt.Color
	HasFg, HasSubFg, HasHeadFg, HasBorderFg, HasBg bool
}

type themeFieldItem struct {
	name             string
	val              string
	dst              *vt.Color
	has              *bool
	allowTransparent bool
}

func parseThemeFields(section string, items []themeFieldItem, t Theme) error {
	for _, item := range items {
		if item.val == "" {
			continue
		}
		*item.has = true
		if item.allowTransparent && isTransparent(item.val) {
			*item.dst = vt.DefaultColor
			continue
		}
		c, err := ParseColorInTheme(item.val, t)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", section, item.name, err)
		}
		*item.dst = c
	}
	return nil
}

// ParseDialogTheme validates a [dialog] colour table against the active theme.
func ParseDialogTheme(d DialogTheme) (DialogStyle, error) {
	return ParseDialogThemeIn(d, activeTheme)
}

// ParseDialogThemeIn validates a [dialog] colour table against t.
func ParseDialogThemeIn(d DialogTheme, t Theme) (DialogStyle, error) {
	var s DialogStyle
	err := parseThemeFields("dialog", []themeFieldItem{
		{"fg", d.Fg, &s.Fg, &s.HasFg, false},
		{"sub_fg", d.SubFg, &s.SubFg, &s.HasSubFg, false},
		{"head_fg", d.HeadFg, &s.HeadFg, &s.HasHeadFg, false},
		{"border_fg", d.BorderFg, &s.BorderFg, &s.HasBorderFg, false},
		{"bg", d.Bg, &s.Bg, &s.HasBg, true},
	}, t)
	return s, err
}

// Apply overrides the active theme's dialog/overlay styles with any set fields.
func (s DialogStyle) Apply() {
	if s.HasBorderFg {
		styleBox.Fg = s.BorderFg
	}
	if s.HasHeadFg {
		styleBoxHead.Fg = s.HeadFg
	}
	if s.HasFg {
		styleBoxKey.Fg = s.Fg
	}
	if s.HasSubFg {
		styleBoxText.Fg = s.SubFg
	}
	if s.HasBg {
		styleBox.Bg = s.Bg
		styleBoxHead.Bg = s.Bg
		styleBoxKey.Bg = s.Bg
		styleBoxText.Bg = s.Bg
	}
}

// FinderTheme holds optional colour overrides for the fuzzy tree picker.
type FinderTheme struct {
	Fg       string `toml:"fg"`
	SubFg    string `toml:"sub_fg"`
	HeadFg   string `toml:"head_fg"`
	BorderFg string `toml:"border_fg"`
	Bg       string `toml:"bg"`
	SelFg    string `toml:"sel_fg"`
	SelBg    string `toml:"sel_bg"`
}

// FinderStyle is a validated set of finder colour overrides.
type FinderStyle struct {
	Fg, SubFg, HeadFg, BorderFg, Bg, SelFg, SelBg          vt.Color
	HasFg, HasSubFg, HasHeadFg, HasBorderFg, HasBg, HasSelFg, HasSelBg bool
}

// ParseFinderTheme validates a [finder] colour table against the active theme.
func ParseFinderTheme(d FinderTheme) (FinderStyle, error) {
	return ParseFinderThemeIn(d, activeTheme)
}

// ParseFinderThemeIn validates a [finder] colour table against t.
func ParseFinderThemeIn(d FinderTheme, t Theme) (FinderStyle, error) {
	var s FinderStyle
	err := parseThemeFields("finder", []themeFieldItem{
		{"fg", d.Fg, &s.Fg, &s.HasFg, false},
		{"sub_fg", d.SubFg, &s.SubFg, &s.HasSubFg, false},
		{"head_fg", d.HeadFg, &s.HeadFg, &s.HasHeadFg, false},
		{"border_fg", d.BorderFg, &s.BorderFg, &s.HasBorderFg, false},
		{"bg", d.Bg, &s.Bg, &s.HasBg, true},
		{"sel_fg", d.SelFg, &s.SelFg, &s.HasSelFg, false},
		{"sel_bg", d.SelBg, &s.SelBg, &s.HasSelBg, true},
	}, t)
	return s, err
}

// Apply overrides the finder's styles with any set fields.
func (s FinderStyle) Apply() {
	if s.HasBorderFg {
		styleFinder.Fg = s.BorderFg
	}
	if s.HasHeadFg {
		styleFinderHead.Fg = s.HeadFg
	}
	if s.HasFg {
		styleFinderKey.Fg = s.Fg
	}
	if s.HasSubFg {
		styleFinderText.Fg = s.SubFg
	}
	if s.HasBg {
		styleFinder.Bg = s.Bg
		styleFinderHead.Bg = s.Bg
		styleFinderKey.Bg = s.Bg
		styleFinderText.Bg = s.Bg
	}
	if s.HasSelFg {
		styleFinderSel.Fg = s.SelFg
	}
	if s.HasSelBg {
		styleFinderSel.Bg = s.SelBg
	}
}

// StackTheme holds optional colour overrides for stacked pane headers.
type StackTheme struct {
	Fg       string `toml:"fg"`
	Bg       string `toml:"bg"`
	ActiveFg string `toml:"active_fg"`
	ActiveBg string `toml:"active_bg"`
}

// StackStyle is a validated set of stacked header colour overrides.
type StackStyle struct {
	Fg, Bg, ActiveFg, ActiveBg          vt.Color
	HasFg, HasBg, HasActiveFg, HasActiveBg bool
}

// ParseStackTheme validates a [stack] colour table against the active theme.
func ParseStackTheme(d StackTheme) (StackStyle, error) {
	return ParseStackThemeIn(d, activeTheme)
}

// ParseStackThemeIn validates a [stack] colour table against t.
func ParseStackThemeIn(d StackTheme, t Theme) (StackStyle, error) {
	var s StackStyle
	err := parseThemeFields("stack", []themeFieldItem{
		{"fg", d.Fg, &s.Fg, &s.HasFg, false},
		{"bg", d.Bg, &s.Bg, &s.HasBg, true},
		{"active_fg", d.ActiveFg, &s.ActiveFg, &s.HasActiveFg, false},
		{"active_bg", d.ActiveBg, &s.ActiveBg, &s.HasActiveBg, true},
	}, t)
	return s, err
}

// Apply overrides stacked header styles with any set fields.
func (s StackStyle) Apply() {
	if s.HasFg {
		styleStack.Fg = s.Fg
	}
	if s.HasBg {
		styleStack.Bg = s.Bg
	}
	if s.HasActiveFg {
		styleStackActive.Fg = s.ActiveFg
	}
	if s.HasActiveBg {
		styleStackActive.Bg = s.ActiveBg
	}
}

// named are the colour names every terminal agrees on.
var named = map[string]uint8{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4,
	"magenta": 5, "cyan": 6, "white": 7,
	"bright-black": 8, "bright-red": 9, "bright-green": 10, "bright-yellow": 11,
	"bright-blue": 12, "bright-magenta": 13, "bright-cyan": 14, "bright-white": 15,
	"grey": 8, "gray": 8,
}

// ParseColor reads a colour written as a name, a 0-255 palette index, or
// #rrggbb.
func ParseColor(s string) (vt.Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, ok := named[s]; ok {
		return vt.Indexed(n), nil
	}
	if hex, ok := strings.CutPrefix(s, "#"); ok {
		if len(hex) != 6 {
			return 0, fmt.Errorf("%q: a hex colour looks like #1a2b3c", s)
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return 0, fmt.Errorf("%q: not a hex colour", s)
		}
		return vt.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return 0, fmt.Errorf("%d: a palette index is 0-255", n)
		}
		return vt.Indexed(uint8(n)), nil
	}
	return 0, fmt.Errorf("%q: expected a name, 0-255 or #rrggbb", s)
}
