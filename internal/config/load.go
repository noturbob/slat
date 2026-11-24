package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/ui"
)

// Config holds the full application configuration.
type Config struct {
	Prefix     string            `toml:"prefix"`
	Shell      string            `toml:"shell"`
	StatusBar         bool              `toml:"status_bar"`
	AutoRename        bool              `toml:"auto_rename"`         // rename tabs after the active pane's program
	RandomSessionName bool              `toml:"random_session_name"` // Diablo-style random session names
	Scrollback        int               `toml:"scrollback"`          // lines of history per pane
	Animate    Duration          `toml:"animate"`    // shorthand: one duration for every animation
	Animations bool              `toml:"animations"` // false turns them all off
	Mouse      bool              `toml:"mouse"`      // false hands the mouse back to the terminal
	Restore    bool              `toml:"restore"`    // bring the session back after a reboot
	Animation  Animation         `toml:"animation"`
	Splash     Splash            `toml:"splash"`
	Dialog     ui.DialogTheme    `toml:"dialog"`
	Finder     ui.FinderTheme    `toml:"finder"`
	Stack      ui.StackTheme     `toml:"stack"`
	Keybinds   map[string]string `toml:"keybinds"`
	Shortcuts  map[string]string `toml:"shortcuts"`
	Theme      map[string]string `toml:"-"`
	ThemeVars  map[string]string `toml:"-"`
	UserThemes map[string]ui.Theme `toml:"-"`
	Borders    map[string]string `toml:"borders"`
	Status     ui.StatusConfig   `toml:"status"`
	Copy       Copy              `toml:"copy"`
	Agent      Agent             `toml:"agent"`
	Navigation Navigation        `toml:"navigation"`

	PrefixKey input.Prefix `toml:"-"` // parsed Prefix
	Path      string       `toml:"-"` // source config file path
}

// Navigation configures direct, prefix-less pane navigation (e.g. Alt+h/j/k/l)
// with passthrough for editors like Neovim that handle split navigation themselves.
type Navigation struct {
	Keys string   `toml:"keys"` // "alt" (default), "ctrl", or "off"
	Apps []string `toml:"apps"` // foreground process names to pass keys through to
}

// Agent tunes how slat reports what a pane is doing, for `slat status`,
// `slat wait` and the notification hooks. See docs/design/agent-cli.md.
type Agent struct {
	// Settle is how long a pane must be quiet, with its shell in the
	// foreground, before it counts as idle.
	Settle Duration `toml:"settle"`
	// InputAfter is how long a running program must be quiet, showing a
	// prompt-like last line, before it counts as waiting for input.
	InputAfter Duration `toml:"input_after"`
	// InputPatterns are the regexps that make a last line look like a
	// question.
	InputPatterns []string `toml:"input_patterns"`
	// OnInput and OnIdle run when a pane enters that state: %p pane,
	// %t tab, %s status, %c the pane's last line.
	OnInput string `toml:"on_input"`
	OnIdle  string `toml:"on_idle"`

	Patterns []*regexp.Regexp `toml:"-"` // compiled InputPatterns
}

// Splash customizes the startup banner shown when the first client attaches.
type Splash struct {
	Enabled     bool     `toml:"enabled"`
	Duration    Duration `toml:"duration"`
	Art         []string `toml:"art"`
	Subtitle    string   `toml:"subtitle"`
	Hint        string   `toml:"hint"`
	Fg          string   `toml:"fg"`
	SubFg       string   `toml:"sub_fg"`
	Bg          string   `toml:"bg"`
	HasSubtitle bool     `toml:"-"`
	HasHint     bool     `toml:"-"`
}

func defaultSplash() Splash {
	return Splash{
		Enabled:  true,
		Duration: Duration(700 * time.Millisecond),
	}
}

// Copy is how yanked text leaves slat.
type Copy struct {
	// OSC52 asks the terminal to put the text on the system clipboard.
	// It is the only method that works over ssh, since the terminal in
	// front of the user is the one holding the clipboard.
	OSC52 bool `toml:"osc52"`
	// Command is an optional local program to pipe the text into as well
	// — wl-copy, xclip -sel clip, pbcopy — for terminals that refuse
	// OSC 52.
	Command string `toml:"command"`
}

// Duration is a time.Duration written as a TOML string ("750ms").
type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	cfg := &Config{
		Prefix:    "C-s",
		PrefixKey: input.Prefix{Name: "Ctrl-S", Bytes: []byte{0x13}},
		Shell:      defaultShell(),
		StatusBar:  true,
		AutoRename: true,
		Scrollback: 2000,
		Animations: true,
		Mouse:      true,
		Restore:    true,
		Animation:  defaultAnimation(),
		Splash:     defaultSplash(),
		Status:     ui.DefaultStatusConfig(),
		Copy:       Copy{OSC52: true},
		Keybinds:   defaultKeybinds(),
		Agent:      defaultAgent(),
		Navigation: defaultNavigation(),
	}
	if err := cfg.Agent.compile(); err != nil { // the built-in patterns are known good
		panic(err)
	}
	return cfg
}

func defaultNavigation() Navigation {
	return Navigation{
		Keys: "alt",
		Apps: []string{"nvim", "vim"},
	}
}

// defaultAgent covers the prompts common tools stop on.
func defaultAgent() Agent {
	return Agent{
		Settle:     Duration(750 * time.Millisecond),
		InputAfter: Duration(10 * time.Second),
		InputPatterns: []string{
			`\[[yY]/[nN]\]`, `\([yY]/[nN]\)`, `(?i)press (enter|any key)`,
			`(?i)\bcontinue\?`, `(?i)(password|passphrase).*:\s*$`,
			`(?i)^\s*(\[\?\]|\?)\s+\S`, `\?\s*$`,
		},
	}
}

func defaultKeybinds() map[string]string {
	return map[string]string{
		// Panes
		"split-horizontal":  "h",
		"split-vertical":    "v",
		"add-pane":          "a",
		"next-pane":         "o",
		"prev-pane":         "O",
		"select-pane-up":    "k",
		"select-pane-down":  "j",
		"select-pane-left":  "H",
		"select-pane-right": "L",
		"swap-pane":         "s",
		"move-mode":         "m",
		"move-pane-up":      "K",
		"move-pane-down":    "J",
		"move-pane-left":    "<",
		"move-pane-right":   ">",
		"resize-grow":       "+",
		"resize-shrink":     "-",
		"equalize":          "=",
		"layout-picker":     "l",
		"close-pane":        "x",
		"zoom":              "z",
		"identify-panes":    "i",
		"stack-mode":        "t",
		"break-pane":        "!",
		"join-pane":         "@",
		// Tabs
		"new-tab":    "c",
		"next-tab":   "n",
		"prev-tab":   "p",
		"rename-tab": ",",
		"close-tab":  "X",
		// Workspaces
		"new-workspace":    "W",
		"next-workspace":   "w",
		"prev-workspace":   "P",
		"rename-workspace": "$",
		// Session
		"rename-session": "S",
		"tree-picker":    "f",
		"session-picker": "F",
		"next-session":   ")",
		"prev-session":   "(",
		"global-tree":       "g",
		"save-session":      "E",
		"toggle-status-bar": "b",
		"detach":            "d",
		"quit":           "q",
		// Scrollback
		"scroll-mode": "[",
	}
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		// $SHELL on Windows is often an MSYS path that CreateProcess can't
		// run, so prefer the console shell. Set shell = "powershell.exe"
		// in the config for something else.
		if s := os.Getenv("COMSPEC"); s != "" {
			return s
		}
		return "cmd.exe"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

func configDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "slat")
}

// Path returns the config file location, honoring $XDG_CONFIG_HOME.
func Path() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.toml")
}

// ThemesDir returns the directory for user-defined *.toml themes.
func ThemesDir() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "themes")
}

// Load reads the default config file, falling back to defaults if it does
// not exist. A config that can't be honored exactly is an error.
func Load() (*Config, error) {
	return loadFile(Path(), true)
}

// LoadFrom reads the config file at path, or falls back to Load() when path is
// empty. Unlike Load, an explicit non-empty path that is missing is an error.
func LoadFrom(path string) (*Config, error) {
	if strings.TrimSpace(path) == "" {
		return Load()
	}
	return loadFile(path, false)
}

type decodeConfig struct {
	Config
	RawTheme map[string]toml.Primitive `toml:"theme"`
}

func unpackThemeTable(md toml.MetaData, raw map[string]toml.Primitive) (settings, vars map[string]string, err error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	settings = make(map[string]string, len(raw))
	for k, prim := range raw {
		if k == "vars" {
			if err := md.PrimitiveDecode(prim, &vars); err != nil {
				return nil, nil, fmt.Errorf("theme.vars: %w", err)
			}
			continue
		}
		var s string
		if err := md.PrimitiveDecode(prim, &s); err != nil {
			return nil, nil, fmt.Errorf("theme.%s: %w", k, err)
		}
		settings[k] = s
	}
	return settings, vars, nil
}

func loadFile(path string, allowMissing bool) (*Config, error) {
	cfg := DefaultConfig()
	cfg.Path = path
	userThemes, err := ui.LoadThemesFromDir(ThemesDir())
	if err != nil {
		return nil, err
	}
	cfg.UserThemes = userThemes
	if path == "" {
		return cfg, nil
	}
	user := &decodeConfig{Config: Config{StatusBar: true, Animations: true, AutoRename: true}}
	md, err := toml.DecodeFile(path, user)
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	settings, vars, err := unpackThemeTable(md, user.RawTheme)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	user.Config.Theme = settings
	user.Config.ThemeVars = vars
	if mergeErr := cfg.merge(&user.Config, md); mergeErr != nil {
		return nil, fmt.Errorf("%s: %w", path, mergeErr)
	}
	cfg.Path = path
	return cfg, nil
}

func (cfg *Config) merge(user *Config, md toml.MetaData) error {
	if len(md.Undecoded()) > 0 {
		return fmt.Errorf("unknown setting %q", md.Undecoded()[0].String())
	}
	if user.Prefix != "" {
		p, err := input.ParsePrefix(user.Prefix)
		if err != nil {
			return err
		}
		cfg.Prefix, cfg.PrefixKey = user.Prefix, p
	}
	if user.Shell != "" {
		cfg.Shell = user.Shell
	}
	cfg.StatusBar = user.StatusBar
	cfg.AutoRename = user.AutoRename
	if err := cfg.mergeAgent(user.Agent, md); err != nil {
		return err
	}
	// Parsed here rather than at draw time, so a bad colour is reported
	// when the user runs slat instead of painting something odd.
	if len(user.Theme) > 0 {
		cfg.Theme = user.Theme
	}
	if len(user.ThemeVars) > 0 {
		cfg.ThemeVars = user.ThemeVars
	}
	theme, err := ui.ParseThemeWith(cfg.Theme, cfg.ThemeVars, cfg.UserThemes)
	if err != nil {
		return err
	}
	if len(user.Borders) > 0 {
		if _, err := ui.ParseBorders(user.Borders); err != nil {
			return err
		}
		cfg.Borders = user.Borders
	}
	if err := cfg.mergeStatus(user.Status, md, theme); err != nil {
		return err
	}
	if err := cfg.mergeAnimation(user, md); err != nil {
		return err
	}
	if err := cfg.mergeSplash(user.Splash, md, theme); err != nil {
		return err
	}
	if _, err := ui.ParseDialogThemeIn(user.Dialog, theme); err != nil {
		return err
	}
	cfg.Dialog = user.Dialog
	if _, err := ui.ParseFinderThemeIn(user.Finder, theme); err != nil {
		return err
	}
	cfg.Finder = user.Finder
	if _, err := ui.ParseStackThemeIn(user.Stack, theme); err != nil {
		return err
	}
	cfg.Stack = user.Stack
	if md.IsDefined("copy", "osc52") {
		cfg.Copy.OSC52 = user.Copy.OSC52
	}
	if md.IsDefined("copy", "command") {
		cfg.Copy.Command = user.Copy.Command
	}
	if md.IsDefined("mouse") {
		cfg.Mouse = user.Mouse
	}
	if md.IsDefined("restore") {
		cfg.Restore = user.Restore
	}
	if md.IsDefined("random_session_name") {
		cfg.RandomSessionName = user.RandomSessionName
	}
	if md.IsDefined("scrollback") {
		if n := user.Scrollback; n < 0 || n > 1_000_000 {
			return fmt.Errorf("scrollback = %d: must be between 0 and 1000000", n)
		}
		cfg.Scrollback = user.Scrollback
	}

	known := map[string]bool{}
	for _, b := range input.Bindings {
		known[b.Name] = true
	}
	// The user's bindings win: a default whose key the user has given to
	// another command is dropped rather than left fighting over the key.
	taken := map[string]string{}
	names := make([]string, 0, len(user.Keybinds))
	for name := range user.Keybinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		key := user.Keybinds[name]
		if !known[name] {
			return fmt.Errorf("unknown keybind %q", name)
		}
		if key == "" {
			continue // explicitly unbound
		}
		if len(key) != 1 {
			return fmt.Errorf("keybind %s = %q: must be a single character", name, key)
		}
		if other, ok := taken[key]; ok {
			return fmt.Errorf("key %q is bound to both %s and %s", key, other, name)
		}
		taken[key] = name
	}
	for name, key := range cfg.Keybinds {
		if _, set := user.Keybinds[name]; set {
			cfg.Keybinds[name] = user.Keybinds[name]
		} else if _, clash := taken[key]; clash {
			delete(cfg.Keybinds, name)
		}
	}
	if len(user.Shortcuts) > 0 {
		h := input.NewHandler(cfg.PrefixKey, nil)
		if err := h.SetShortcuts(user.Shortcuts); err != nil {
			return err
		}
		cfg.Shortcuts = user.Shortcuts
	}
	if md.IsDefined("navigation", "keys") {
		k := strings.ToLower(strings.TrimSpace(user.Navigation.Keys))
		switch k {
		case "", "off", "alt", "ctrl":
			cfg.Navigation.Keys = k
		default:
			return fmt.Errorf("navigation.keys = %q: must be \"alt\", \"ctrl\", or \"off\"", user.Navigation.Keys)
		}
	}
	if md.IsDefined("navigation", "apps") {
		cfg.Navigation.Apps = user.Navigation.Apps
	}
	return nil
}

// mergeAgent applies the [agent] table and compiles its patterns.
func (cfg *Config) mergeAgent(user Agent, md toml.MetaData) error {
	if md.IsDefined("agent", "settle") {
		cfg.Agent.Settle = user.Settle
	}
	if md.IsDefined("agent", "input_after") {
		cfg.Agent.InputAfter = user.InputAfter
	}
	if md.IsDefined("agent", "input_patterns") {
		cfg.Agent.InputPatterns = user.InputPatterns
	}
	cfg.Agent.OnInput, cfg.Agent.OnIdle = user.OnInput, user.OnIdle
	return cfg.Agent.compile()
}

func (cfg *Config) mergeStatus(user ui.StatusConfig, md toml.MetaData, theme ui.Theme) error {
	strings := []struct {
		key string
		dst *string
		src string
	}{
		{"left", &cfg.Status.Left, user.Left},
		{"right", &cfg.Status.Right, user.Right},
		{"tab", &cfg.Status.Tab, user.Tab},
		{"tab_active", &cfg.Status.TabActive, user.TabActive},
		{"tab_sep", &cfg.Status.TabSep, user.TabSep},
		{"alert", &cfg.Status.Alert, user.Alert},
		{"prefix", &cfg.Status.Prefix, user.Prefix},
		{"prefix_active", &cfg.Status.PrefixActive, user.PrefixActive},
		{"zoom", &cfg.Status.Zoom, user.Zoom},
		{"tab_zoom_glyph", &cfg.Status.TabZoomGlyph, user.TabZoomGlyph},
		{"pane_id", &cfg.Status.PaneID, user.PaneID},
		{"pane_id_active", &cfg.Status.PaneIDActive, user.PaneIDActive},
	}
	for _, f := range strings {
		if md.IsDefined("status", f.key) {
			*f.dst = f.src
		}
	}
	lists := []struct {
		key string
		dst *[]string
		src []string
	}{
		{"tab_colors", &cfg.Status.TabColors, user.TabColors},
		{"pane_colors", &cfg.Status.PaneColors, user.PaneColors},
		{"tab_glyphs", &cfg.Status.TabGlyphs, user.TabGlyphs},
		{"tab_index_glyphs", &cfg.Status.TabIndexGlyphs, user.TabIndexGlyphs},
		{"tab_index_zoom_glyphs", &cfg.Status.TabIndexZoomGlyphs, user.TabIndexZoomGlyphs},
		{"pane_glyphs", &cfg.Status.PaneGlyphs, user.PaneGlyphs},
		{"pane_zoom_glyphs", &cfg.Status.PaneZoomGlyphs, user.PaneZoomGlyphs},
		{"current_pane_glyphs", &cfg.Status.CurrentPaneGlyphs, user.CurrentPaneGlyphs},
		{"current_pane_zoom_glyphs", &cfg.Status.CurrentPaneZoomGlyphs, user.CurrentPaneZoomGlyphs},
		{"current_pane_id_glyphs", &cfg.Status.CurrentPaneIDGlyphs, user.CurrentPaneIDGlyphs},
		{"current_pane_id_zoom_glyphs", &cfg.Status.CurrentPaneIDZoomGlyphs, user.CurrentPaneIDZoomGlyphs},
	}
	for _, f := range lists {
		if md.IsDefined("status", f.key) {
			*f.dst = f.src
		}
	}
	zoomFlags := []struct {
		key string
		dst *bool
		src bool
	}{
		{"tab", &cfg.Status.ZoomGlyphs.Tab, user.ZoomGlyphs.Tab},
		{"tab_index", &cfg.Status.ZoomGlyphs.TabIndex, user.ZoomGlyphs.TabIndex},
		{"pane", &cfg.Status.ZoomGlyphs.Pane, user.ZoomGlyphs.Pane},
		{"current_pane", &cfg.Status.ZoomGlyphs.CurrentPane, user.ZoomGlyphs.CurrentPane},
		{"current_pane_id", &cfg.Status.ZoomGlyphs.CurrentPaneID, user.ZoomGlyphs.CurrentPaneID},
	}
	for _, f := range zoomFlags {
		if md.IsDefined("status", "zoom_glyphs", f.key) {
			*f.dst = f.src
		}
	}
	if md.IsDefined("status", "process_glyphs") {
		cfg.Status.ProcessGlyphs = user.ProcessGlyphs
	}
	if md.IsDefined("status", "scripts") {
		cfg.Status.Scripts = user.Scripts
	}
	_, err := ui.ParseStatusConfigIn(cfg.Status, theme)
	return err
}

func (cfg *Config) mergeSplash(user Splash, md toml.MetaData, theme ui.Theme) error {
	if md.IsDefined("splash", "enabled") {
		cfg.Splash.Enabled = user.Enabled
	}
	if md.IsDefined("splash", "duration") {
		if user.Duration.D() < 0 {
			return errors.New("splash.duration must not be negative")
		}
		cfg.Splash.Duration = user.Duration
	}
	if md.IsDefined("splash", "art") {
		cfg.Splash.Art = user.Art
	}
	if md.IsDefined("splash", "subtitle") {
		cfg.Splash.Subtitle = user.Subtitle
		cfg.Splash.HasSubtitle = true
	}
	if md.IsDefined("splash", "hint") {
		cfg.Splash.Hint = user.Hint
		cfg.Splash.HasHint = true
	}
	if md.IsDefined("splash", "fg") {
		cfg.Splash.Fg = user.Fg
	}
	if md.IsDefined("splash", "sub_fg") {
		cfg.Splash.SubFg = user.SubFg
	}
	if md.IsDefined("splash", "bg") {
		cfg.Splash.Bg = user.Bg
	}
	_, err := ui.ParseSplashIn(cfg.Splash.Art, cfg.Splash.Subtitle, cfg.Splash.Hint, cfg.Splash.Fg, cfg.Splash.SubFg, cfg.Splash.Bg, theme)
	return err
}

func (a *Agent) compile() error {
	a.Patterns = nil
	for _, p := range a.InputPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("agent.input_patterns: %w", err)
		}
		a.Patterns = append(a.Patterns, re)
	}
	if a.Settle.D() < 0 || a.InputAfter.D() < 0 {
		return errors.New("agent.settle and agent.input_after must not be negative")
	}
	return nil
}
