package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/noturbob/slat/internal/input"
)

// Config holds the full application configuration.
type Config struct {
	Prefix    string            `toml:"prefix"`
	Shell     string            `toml:"shell"`
	StatusBar bool              `toml:"status_bar"`
	Keybinds  map[string]string `toml:"keybinds"`

	PrefixByte byte `toml:"-"` // parsed Prefix
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Prefix:     "C-s",
		PrefixByte: 0x13,
		Shell:      defaultShell(),
		StatusBar:  true,
		Keybinds:   defaultKeybinds(),
	}
}

func defaultKeybinds() map[string]string {
	return map[string]string{
		// Panes
		"split-horizontal":  "h",
		"split-vertical":    "v",
		"next-pane":         "o",
		"prev-pane":         "O",
		"select-pane-up":    "k",
		"select-pane-down":  "j",
		"select-pane-left":  "H",
		"select-pane-right": "L",
		"swap-pane":         "s",
		"resize-grow":       "+",
		"resize-shrink":     "-",
		"equalize":          "=",
		"close-pane":        "x",
		"zoom":              "z",
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
		"detach": "d",
		"quit":   "q",
	}
}

func defaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Path returns the config file location, honoring $XDG_CONFIG_HOME.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "slat", "config.toml")
}

// Load reads the config file, falling back to defaults for anything unset.
// A config that can't be honored exactly is an error, not a silent guess.
func Load() (*Config, error) {
	cfg := DefaultConfig()
	path := Path()
	if path == "" {
		return cfg, nil
	}
	user := &Config{StatusBar: true}
	md, err := toml.DecodeFile(path, user)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.merge(user, md); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func (cfg *Config) merge(user *Config, md toml.MetaData) error {
	if len(md.Undecoded()) > 0 {
		return fmt.Errorf("unknown setting %q", md.Undecoded()[0].String())
	}
	if user.Prefix != "" {
		b, err := input.ParsePrefix(user.Prefix)
		if err != nil {
			return err
		}
		cfg.Prefix, cfg.PrefixByte = user.Prefix, b
	}
	if user.Shell != "" {
		cfg.Shell = user.Shell
	}
	cfg.StatusBar = user.StatusBar

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
	return nil
}
