package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/ui"
)

const configReloadInterval = 500 * time.Millisecond

type compiledConfig struct {
	theme   ui.Theme
	dlg     ui.DialogStyle
	fnd     ui.FinderStyle
	stk     ui.StackStyle
	borders ui.BorderSet
	status  ui.StatusFormat
	splash  ui.Splash
	help    []ui.HelpEntry
	handler *input.Handler
}

func compileConfig(cfg *config.Config) (compiledConfig, error) {
	if cfg == nil {
		return compiledConfig{}, errors.New("config must not be nil")
	}
	theme, err := ui.ParseThemeWith(cfg.Theme, cfg.ThemeVars, cfg.UserThemes)
	if err != nil {
		return compiledConfig{}, err
	}
	dlg, err := ui.ParseDialogThemeIn(cfg.Dialog, theme)
	if err != nil {
		return compiledConfig{}, err
	}
	fnd, err := ui.ParseFinderThemeIn(cfg.Finder, theme)
	if err != nil {
		return compiledConfig{}, err
	}
	stk, err := ui.ParseStackThemeIn(cfg.Stack, theme)
	if err != nil {
		return compiledConfig{}, err
	}
	borders, err := ui.ParseBorders(cfg.Borders)
	if err != nil {
		return compiledConfig{}, err
	}
	status, err := ui.ParseStatusConfigIn(cfg.Status, theme)
	if err != nil {
		return compiledConfig{}, err
	}
	prefixName := cfg.PrefixKey.Name
	subtitle := "terminal multiplexer " + Version
	if cfg.Splash.HasSubtitle {
		subtitle = strings.ReplaceAll(cfg.Splash.Subtitle, "{version}", Version)
	}
	hint := fmt.Sprintf("press %s then ? for help", prefixName)
	if cfg.Splash.HasHint {
		hint = strings.ReplaceAll(cfg.Splash.Hint, "{prefix}", prefixName)
	}
	splash, err := ui.ParseSplashIn(cfg.Splash.Art, subtitle, hint, cfg.Splash.Fg, cfg.Splash.SubFg, cfg.Splash.Bg, theme)
	if err != nil {
		return compiledConfig{}, err
	}
	handler := input.NewHandler(cfg.PrefixKey, cfg.Keybinds)
	if scErr := handler.SetShortcuts(cfg.Shortcuts); scErr != nil {
		return compiledConfig{}, scErr
	}
	return compiledConfig{
		theme:   theme,
		dlg:     dlg,
		fnd:     fnd,
		stk:     stk,
		borders: borders,
		status:  status,
		splash:  splash,
		help:    helpEntries(cfg),
		handler: handler,
	}, nil
}

func (cc compiledConfig) applyStyles() {
	cc.theme.Apply()
	cc.dlg.Apply()
	cc.fnd.Apply()
	cc.stk.Apply()
	cc.borders.Apply()
}

// ApplyConfig validates and applies cfg to the running App.
func (a *App) ApplyConfig(cfg *config.Config) error {
	cc, err := compileConfig(cfg)
	if err != nil {
		return err
	}
	a.mu.Lock()
	cc.applyStyles()
	a.cfg = cfg
	a.status = cc.status
	a.splash = cc.splash
	a.help = cc.help
	a.handler = cc.handler
	if a.manager != nil {
		a.manager.UpdateDefaults(cfg.Shell, cfg.Scrollback)
	}
	if a.screen != nil {
		a.screen.Invalidate()
	}
	a.mu.Unlock()
	a.markDirty()
	return nil
}

// ReloadConfig reloads configuration from the active config path and applies it.
func (a *App) ReloadConfig() error {
	a.mu.Lock()
	cfgPath := a.cfg.Path
	a.mu.Unlock()
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return err
	}
	return a.ApplyConfig(cfg)
}

func (a *App) configReloadLoop() {
	a.mu.Lock()
	cfgPath := a.cfg.Path
	a.mu.Unlock()
	if cfgPath == "" {
		return
	}
	lastSig := configSnapshot(cfgPath, config.ThemesDir())
	tick := time.NewTicker(configReloadInterval)
	defer tick.Stop()
	for {
		select {
		case <-a.quit:
			return
		case <-tick.C:
		}
		a.mu.Lock()
		cfgPath = a.cfg.Path
		a.mu.Unlock()
		if cfgPath == "" {
			continue
		}
		sig := configSnapshot(cfgPath, config.ThemesDir())
		if sig == lastSig {
			continue
		}
		lastSig = sig
		if err := a.ReloadConfig(); err != nil {
			a.notify("config reload failed: " + err.Error())
		} else {
			a.notify("config reloaded")
		}
	}
}

func configSnapshot(cfgPath, themesDir string) string {
	var b strings.Builder
	b.WriteString(fileFingerprint(cfgPath))
	if themesDir == "" {
		return b.String()
	}
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return b.String()
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		b.WriteString(fileFingerprint(filepath.Join(themesDir, entry.Name())))
	}
	return b.String()
}

func fileFingerprint(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path + ":missing;"
	}
	return fmt.Sprintf("%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())
}
