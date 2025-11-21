package config

import (
	"os"
	"path/filepath"
	"github.com/BurntSushi/toml"
)

type Config struct {
	Prefix string `toml:"prefix"`
}

func Load() (*Config, error) {
	cfg := &Config{Prefix: "C-s"}
	home, err := os.UserHomeDir()
	if err != nil { return nil, err }
	path := filepath.Join(home, ".config", "slat", "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) { return cfg, nil }
	toml.DecodeFile(path, cfg)
	return cfg, nil
}