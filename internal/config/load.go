package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds the application's configuration values.
type Config struct {
	// Prefix is the key combination used to trigger commands.
	Prefix string `toml:"prefix"`
}

// default_config returns a Config struct with sensible default values.
func default_config() *Config {
	return &Config{
		Prefix: "C-s", // Ctrl+s
	}
}

// Load finds, parses, and returns the configuration. If no config file is found,
// it returns the default configuration.
func Load() (*Config, error) {
	// Start with the defaults.
	cfg := default_config()

	// Determine the config file path, typically ~/.config/slat/config.toml
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(homeDir, ".config", "slat", "config.toml")

	// If the config file doesn't exist, just return the defaults.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return cfg, nil
	}

	// If the file does exist, read and parse it.
	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}