package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the full application configuration.
type Config struct {
	Prefix    string            `toml:"prefix"`
	Shell     string            `toml:"shell"`
	StatusBar bool              `toml:"status_bar"`
	Keybinds  map[string]string `toml:"keybinds"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Prefix:    "C-s",
		Shell:     defaultShell(),
		StatusBar: true,
		Keybinds:  defaultKeybinds(),
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

// Load reads the config from ~/.config/slat/config.toml, falling back to defaults.
func Load() (*Config, error) {
	cfg := DefaultConfig()
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // use defaults
	}
	path := filepath.Join(home, ".config", "slat", "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config parse error: %w", err)
	}

	// BUG FIX: if the user's config.toml has no [keybinds] table at all,
	// toml.DecodeFile leaves cfg.Keybinds nil. Writing into a nil map below
	// would panic on startup. Initialize it first.
	if cfg.Keybinds == nil {
		cfg.Keybinds = map[string]string{}
	}

	// Fill in any missing keybinds with defaults
	defaults := defaultKeybinds()
	for k, v := range defaults {
		if _, ok := cfg.Keybinds[k]; !ok {
			cfg.Keybinds[k] = v
		}
	}
	if cfg.Shell == "" {
		cfg.Shell = defaultShell()
	}
	// BUG FIX: an empty/whitespace prefix in a malformed config used to
	// silently fall through to a semi-arbitrary default deep in ParsePrefix.
	// Make the fallback explicit here instead.
	if strings.TrimSpace(cfg.Prefix) == "" {
		cfg.Prefix = "C-s"
	}
	return cfg, nil
}