package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/input"
	"github.com/noturbob/slat/internal/ui"
)

type compiledConfig struct {
	theme         ui.Theme
	dlg           ui.DialogStyle
	fnd           ui.FinderStyle
	stk           ui.StackStyle
	borders       ui.BorderSet
	helpBorders   ui.BorderSet
	finderBorders ui.BorderSet
	dialogBorders ui.BorderSet
	status        ui.StatusFormat
	splash        ui.Splash
	help          []ui.HelpEntry
	handler       *input.Handler
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
	helpBorders, err := ui.ParseModalBorders(cfg.Borders, cfg.HelpBorders)
	if err != nil {
		return compiledConfig{}, fmt.Errorf("borders.help: %w", err)
	}
	finderBorders, err := ui.ParseModalBorders(cfg.Borders, cfg.FinderBorders)
	if err != nil {
		return compiledConfig{}, fmt.Errorf("borders.finder: %w", err)
	}
	dialogBorders, err := ui.ParseModalBorders(cfg.Borders, cfg.DialogBorders)
	if err != nil {
		return compiledConfig{}, fmt.Errorf("borders.dialog: %w", err)
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
		theme:         theme,
		dlg:           dlg,
		fnd:           fnd,
		stk:           stk,
		borders:       borders,
		helpBorders:   helpBorders,
		finderBorders: finderBorders,
		dialogBorders: dialogBorders,
		status:        status,
		splash:        splash,
		help:          helpEntries(cfg),
		handler:       handler,
	}, nil
}

func (cc compiledConfig) applyStyles() {
	cc.theme.Apply()
	cc.dlg.Apply()
	cc.fnd.Apply()
	cc.stk.Apply()
	cc.borders.Apply()
	cc.helpBorders.ApplyHelp()
	cc.finderBorders.ApplyFinder()
	cc.dialogBorders.ApplyDialog()
}

// ApplyConfig validates and applies cfg to the running App.
func (a *App) ApplyConfig(cfg *config.Config) error {
	cc, err := compileConfig(cfg)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.applyCompiledLocked(cfg, cc)
	a.mu.Unlock()
	a.markDirty()
	return nil
}

func (a *App) applyCompiledLocked(cfg *config.Config, cc compiledConfig) {
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
}

// reloadConfigLocked reloads configuration from the active config path.
// Caller must hold a.mu.
func (a *App) reloadConfigLocked() error {
	cfg, err := config.LoadFrom(a.cfg.Path)
	if err != nil {
		return err
	}
	cc, err := compileConfig(cfg)
	if err != nil {
		return err
	}
	a.applyCompiledLocked(cfg, cc)
	return nil
}

// ReloadConfig reloads configuration from the active config path and applies it.
func (a *App) ReloadConfig() error {
	a.mu.Lock()
	err := a.reloadConfigLocked()
	a.mu.Unlock()
	if err == nil {
		a.markDirty()
	}
	return err
}
