package config

import (
	"fmt"
	"os"
	"path/filepath"

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
		"split-horizontal": "h",
		"split-vertical":   "v",
		"next-pane":        "o",
		"prev-pane":        "O",
		"close-pane":       "x",
		"new-tab":          "c",
		"next-tab":         "n",
		"prev-tab":         "p",
		"new-workspace":    "W",
		"next-workspace":   "w",
		"detach":           "d",
		"zoom":             "z",
		"quit":             "q",
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

	return cfg, nil
}
