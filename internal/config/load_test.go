package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, toml string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "slat"), 0o755)
	os.WriteFile(filepath.Join(dir, "slat", "config.toml"), []byte(toml), 0o644)
	return Load()
}

func TestUserBindingTakesKeyFromDefault(t *testing.T) {
	// "s" is swap-pane by default; giving it to split-horizontal must
	// unbind swap-pane rather than leave two commands on one key.
	cfg, err := load(t, "prefix = \"C-a\"\n[keybinds]\nsplit-horizontal = \"s\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PrefixKey.Bytes) != 1 || cfg.PrefixKey.Bytes[0] != 0x01 {
		t.Errorf("prefix bytes = %x, want 01", cfg.PrefixKey.Bytes)
	}
	if cfg.Keybinds["split-horizontal"] != "s" {
		t.Errorf("split-horizontal = %q", cfg.Keybinds["split-horizontal"])
	}
	if k, ok := cfg.Keybinds["swap-pane"]; ok {
		t.Errorf("swap-pane still bound to %q", k)
	}
	if !cfg.StatusBar || !cfg.AutoRename || cfg.Keybinds["new-tab"] != "c" {
		t.Error("unset options lost their defaults")
	}
	cfg2, err := load(t, "auto_rename = false\nrandom_session_name = true\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.AutoRename || !cfg2.RandomSessionName {
		t.Errorf("auto_rename=%v random_session_name=%v", cfg2.AutoRename, cfg2.RandomSessionName)
	}
	spCfg, err := load(t, `
[splash]
enabled  = false
duration = "1200ms"
art      = ["  hi  ", " there"]
subtitle = ""
hint     = "custom {prefix}"
fg       = "cyan"
sub_fg   = "248"
bg       = "transparent"
`)
	if err != nil {
		t.Fatal(err)
	}
	if spCfg.Splash.Enabled || spCfg.Splash.Duration.D() != 1200*time.Millisecond ||
		!spCfg.Splash.HasSubtitle || spCfg.Splash.Subtitle != "" ||
		!spCfg.Splash.HasHint || len(spCfg.Splash.Art) != 2 {
		t.Errorf("unexpected splash config: %+v", spCfg.Splash)
	}
	if _, err := load(t, "[splash]\nduration = \"-1s\"\n"); err == nil {
		t.Error("negative splash.duration should fail")
	}
	if _, err := load(t, "[splash]\nfg = \"nope\"\n"); err == nil {
		t.Error("invalid splash.fg should fail")
	}
	dlgCfg, err := load(t, "[dialog]\nfg = \"white\"\nsub_fg = \"248\"\nhead_fg = \"cyan\"\nborder_fg = \"blue\"\nbg = \"#1e1e2e\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if dlgCfg.Dialog.Fg != "white" || dlgCfg.Dialog.Bg != "#1e1e2e" {
		t.Errorf("unexpected dialog config: %+v", dlgCfg.Dialog)
	}
	if _, err := load(t, "[dialog]\nbg = \"nope\"\n"); err == nil {
		t.Error("invalid dialog.bg should fail")
	}
	fsCfg, err := load(t, "[finder]\nsel_bg = \"cyan\"\n[stack]\nactive_fg = \"yellow\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if fsCfg.Finder.SelBg != "cyan" || fsCfg.Stack.ActiveFg != "yellow" {
		t.Errorf("unexpected finder/stack config: %+v %+v", fsCfg.Finder, fsCfg.Stack)
	}
	if _, err := load(t, "[finder]\nsel_fg = \"nope\"\n"); err == nil {
		t.Error("invalid finder.sel_fg should fail")
	}
	if _, err := load(t, "[stack]\nactive_bg = \"nope\"\n"); err == nil {
		t.Error("invalid stack.active_bg should fail")
	}
}

func TestInvalidConfigsAreRejected(t *testing.T) {
	for toml, want := range map[string]string{
		"prefix = \"Ctrl+S\"\n":                     "invalid prefix",
		"prefix = \"C-[\"\n":                        "Escape key",
		"scrollback = -5\n":                         "between 0 and",
		"[keybinds]\nsplit-sideways = \"v\"\n":      "unknown keybind",
		"[keybinds]\nnew-tab = \"ct\"\n":            "single character",
		"[keybinds]\nnew-tab = \"a\"\nquit = \"a\"": "bound to both",
		"statusbar = false\n":                       "unknown setting",
	} {
		if _, err := load(t, toml); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got error %v, want %q", toml, err, want)
		}
	}
}

func TestMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil || len(cfg.PrefixKey.Bytes) != 1 || cfg.PrefixKey.Bytes[0] != 0x13 || len(cfg.Keybinds) != len(defaultKeybinds()) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestFunctionKeyAndCustomPrefix(t *testing.T) {
	cfg, err := load(t, "prefix = \"F13\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrefixKey.Name != "F13" || string(cfg.PrefixKey.Bytes) != "\x1b[25~" {
		t.Errorf("F13 prefix = %+v", cfg.PrefixKey)
	}

	cfg, err = load(t, "prefix = '\\e[25~'\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrefixKey.Name != `\e[25~` || string(cfg.PrefixKey.Bytes) != "\x1b[25~" {
		t.Errorf("custom prefix = %+v", cfg.PrefixKey)
	}
}

func TestExampleConfigLoads(t *testing.T) {
	example, err := os.ReadFile("../../config.example.toml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := load(t, string(example)); err != nil {
		t.Fatalf("config.example.toml: %v", err)
	}
}

func TestAgentSettings(t *testing.T) {
	cfg, err := load(t, `
[agent]
settle = "2s"
input_after = "1m"
input_patterns = ['ready\?']
on_idle = "notify-send done"
`)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Agent
	if a.Settle.D() != 2*time.Second || a.InputAfter.D() != time.Minute {
		t.Errorf("durations: %v, %v", a.Settle.D(), a.InputAfter.D())
	}
	if len(a.Patterns) != 1 || !a.Patterns[0].MatchString("all ready?") {
		t.Errorf("patterns = %v", a.Patterns)
	}
	if a.OnIdle != "notify-send done" {
		t.Errorf("on_idle = %q", a.OnIdle)
	}

	// Unset keys keep the defaults, and bad values are refused.
	cfg, err = load(t, "[agent]\nsettle = \"1s\"\n")
	if err != nil || len(cfg.Agent.Patterns) != len(defaultAgent().InputPatterns) {
		t.Errorf("defaults not kept: %v, %v", cfg.Agent, err)
	}
	if _, err := load(t, "[agent]\nsettle = \"-1s\"\n"); err == nil {
		t.Error("a negative settle should be an error")
	}
	if _, err := load(t, "[agent]\ninput_patterns = ['(']\n"); err == nil {
		t.Error("a bad pattern should be an error")
	}
	if _, err := load(t, "[agent]\nsettle_time = \"1s\"\n"); err == nil {
		t.Error("an unknown agent setting should be an error")
	}
}

func TestAnimationSettings(t *testing.T) {
	cfg, err := load(t, `
[animation]
split  = "50ms"
move   = "200ms"
easing = "linear"
reveal = "none"
glyph  = "▓"
`)
	if err != nil {
		t.Fatal(err)
	}
	a := cfg.Animation
	if a.Split.D() != 50*time.Millisecond || a.Move.D() != 200*time.Millisecond {
		t.Errorf("durations: %v, %v", a.Split.D(), a.Move.D())
	}
	if a.Reveal != "none" || a.RevealGlyph() != '▓' {
		t.Errorf("reveal = %q %q", a.Reveal, a.RevealGlyph())
	}
	// linear leaves progress alone; the default curve does not.
	if got := a.Ease(0.5); got != 0.5 {
		t.Errorf("linear Ease(0.5) = %v", got)
	}
	if got := DefaultConfig().Animation.Ease(0.5); got <= 0.5 {
		t.Errorf("out-cubic Ease(0.5) = %v, want more than half way", got)
	}

	// The old single knob still works, and one switch turns everything off.
	if cfg, err := load(t, `animate = "300ms"`); err != nil ||
		cfg.Animation.Split.D() != 300*time.Millisecond || cfg.Animation.Move.D() != 300*time.Millisecond {
		t.Errorf("animate shorthand: %+v, %v", cfg.Animation, err)
	}
	if cfg, err := load(t, "animations = false"); err != nil ||
		cfg.Animation.Split != 0 || cfg.Animation.Move != 0 {
		t.Errorf("animations = false: %+v, %v", cfg.Animation, err)
	}

	for _, bad := range []string{
		"[animation]\neasing = \"bouncy\"\n",
		"[animation]\nreveal = \"sparkle\"\n",
		"[animation]\nglyph = \"ab\"\n",
		"[animation]\nsplit = \"-1s\"\n",
		"[animation]\nsplit = \"5s\"\n", // long enough to be in the way
		"[animation]\nspin = \"1s\"\n",  // unknown setting
	} {
		if _, err := load(t, bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}
}

func TestBordersAndStatusSettings(t *testing.T) {
	cfg, err := load(t, `
[borders]
style = "rounded"
vertical = "┇"

[status]
left  = "{prefix} {workspace} {tabs}"
right = "{time} "
tab   = "[{tab_glyph}{index}]"
tab_active = "<{tab_glyph}{index}>"
tab_sep = ""
prefix = "{fg=cyan}  {/}"
prefix_active = "{fg=yellow,bold}   {/}"
zoom = "{fg=yellow} ▣{/}"
tab_zoom_glyph = "▣"
tab_colors = ["cyan", "#ff8800", "44"]
pane_colors = ["red", "208"]
tab_glyphs = ["🐟", "🦀"]
tab_index_glyphs = ["⓪", "①", "②"]
pane_glyphs = ["₀", "₁", "₂"]
pane_zoom_glyphs = ["⓿", "❶", "❷"]
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Borders["style"] != "rounded" || cfg.Status.Tab != "[{tab_glyph}{index}]" ||
		cfg.Status.TabActive != "<{tab_glyph}{index}>" || cfg.Status.TabSep != "" {
		t.Errorf("tables not kept: %v %+v", cfg.Borders, cfg.Status)
	}
	if len(cfg.Status.TabColors) != 3 || len(cfg.Status.PaneGlyphs) != 3 ||
		len(cfg.Status.TabIndexGlyphs) != 3 || len(cfg.Status.PaneZoomGlyphs) != 3 {
		t.Errorf("status arrays not kept: %+v", cfg.Status)
	}
	// Unset status fields keep their defaults.
	if cfg.Status.Alert != " ?" {
		t.Errorf("status.Alert = %q, want default %q", cfg.Status.Alert, " ?")
	}

	// Mistakes are refused when slat starts, not drawn oddly later.
	for _, bad := range []string{
		"[borders]\nstyle = \"fancy\"\n",
		"[borders]\nvertical = \"too long\"\n",
		"[borders]\ncorner = \"+\"\n",
		"[status]\nleft = \"{workspaec}\"\n",
		"[status]\nmiddle = \"x\"\n",
		"[status]\ntab_colors = [\"notacolor\"]\n",
		"[status]\nleft = \"{fg=nope}x{/}\"\n",
		"[status]\nright = \"{script:missing}\"\n",
		"[shortcuts]\n\"bad\" = \"echo hi\"\n",
		"[shortcuts]\n\"Alt+1\" = \"\"\n",
	} {
		if _, err := load(t, bad); err == nil {
			t.Errorf("%q should have been refused", bad)
		}
	}

	cfgScripts, err := load(t, `
[shortcuts]
"Alt+1" = "~/.config/slat/scripts/save.sh"

[status]
right = "{script:git} {pane}/{panes} "
pane_id = "{fg=cyan} #{id} {/}"
pane_id_active = "{fg=yellow,bold} #{id} {/}"
tab = " {current_pane_glyph}:{current_pane_id_glyph} "
current_pane_glyphs = ["a", "b"]
current_pane_zoom_glyphs = ["A", "B"]
current_pane_id_glyphs = ["0", "1", "2"]
current_pane_id_zoom_glyphs = ["⓪", "①", "②"]

[status.scripts.git]
command = "git branch --show-current"
interval = "5s"
timeout = "2s"
fallback = "{fg=red}err{/}"
`)
	if err != nil {
		t.Fatalf("valid shortcuts + status scripts failed: %v", err)
	}
	if cfgScripts.Shortcuts["Alt+1"] != "~/.config/slat/scripts/save.sh" {
		t.Errorf("Shortcuts[Alt+1] = %q", cfgScripts.Shortcuts["Alt+1"])
	}
	if cfgScripts.Status.Scripts["git"].Command != "git branch --show-current" {
		t.Errorf("Status.Scripts[git] = %+v", cfgScripts.Status.Scripts["git"])
	}

	if cfgScripts.Navigation.Keys != "alt" {
		t.Errorf("default Navigation.Keys = %q, want alt", cfgScripts.Navigation.Keys)
	}
	cfgNav, err := load(t, "[navigation]\nkeys = \"ctrl\"\napps = [\"nvim\"]\n")
	if err != nil || cfgNav.Navigation.Keys != "ctrl" || len(cfgNav.Navigation.Apps) != 1 {
		t.Errorf("custom navigation = (%+v, %v)", cfgNav.Navigation, err)
	}
	if _, err := load(t, "[navigation]\nkeys = \"bad\"\n"); err == nil {
		t.Errorf("expected error for invalid navigation.keys")
	}
}

// Every rice in the gallery must load: a broken example is worse than none.
func TestGalleryRicesLoad(t *testing.T) {
	rices, err := filepath.Glob("../../docs/rices/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(rices) == 0 {
		t.Fatal("no rices found: did docs/rices move?")
	}
	for _, path := range rices {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := load(t, string(body)); err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}
}

func TestExampleConfigsLoad(t *testing.T) {
	paths, err := filepath.Glob("../../examples/configs/*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 5 {
		t.Fatalf("expected at least 5 example configs, got %d", len(paths))
	}
	paths = append(paths, "../../config.example.toml")
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := load(t, string(body)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if _, err := LoadFrom(path); err != nil {
			t.Errorf("LoadFrom(%s): %v", path, err)
		}
	}
	if _, err := LoadFrom(""); err != nil {
		t.Errorf("LoadFrom(\"\") should fall back to default Load(): %v", err)
	}
	if _, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Errorf("LoadFrom on missing file should have failed")
	}
}
