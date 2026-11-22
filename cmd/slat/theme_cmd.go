package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jxskiss/mcli"
	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/ui"
)

func registerThemeCommands(
	m *mcli.App,
	deps cliDeps,
	checkCmd func(ctx *mcli.Context, maxPositional int) bool,
	fail func(code int, err error),
	parseOpts []mcli.ParseOpt,
	globals *globalFlags,
) {
	catTheme := mcli.WithCategory("Themes")

	resolveThemeData := func(rawName string) ([]byte, string, error) {
		cfg, err := config.LoadFrom(resolveConfigPath(globals.Config))
		if err != nil {
			return nil, "", err
		}
		target := strings.TrimSpace(rawName)
		var customOverride *ui.Theme
		if target == "" {
			target = strings.TrimSpace(cfg.Theme["name"])
			if target == "" {
				target = "default"
			}
			_, hasName := cfg.Theme["name"]
			hasOverrides := len(cfg.ThemeVars) > 0 || len(cfg.Theme) > 1 || (len(cfg.Theme) == 1 && !hasName)
			if hasOverrides {
				th, err := ui.ParseThemeWith(cfg.Theme, cfg.ThemeVars, cfg.UserThemes)
				if err != nil {
					return nil, "", err
				}
				customOverride = &th
			}
		}
		return ui.ExportThemeTOML(target, cfg.UserThemes, customOverride)
	}

	themeNameComp := func(ctx mcli.ArgCompletionContext) []mcli.CompletionItem {
		cfgPath := ""
		if g, ok := ctx.GlobalFlags().(*globalFlags); ok && g != nil {
			cfgPath = resolveConfigPath(g.Config)
		}
		return completeThemeName(ctx.ArgPrefix(), cfgPath)
	}
	themeParseOpts := append(append([]mcli.ParseOpt(nil), parseOpts...), mcli.WithArgCompFuncs(map[string]mcli.ArgCompletionFunc{
		"name": themeNameComp,
	}))

	type themeLsArgs struct {
		JSONFlag
	}
	addCmd(m, "theme ls", "list built-in and user themes", 0, checkCmd, parseOpts,
		func(_ *mcli.Context, args *themeLsArgs) {
			cfg, err := config.LoadFrom(resolveConfigPath(globals.Config))
			if err != nil {
				fail(1, err)
				return
			}
			list := ui.ListThemes(cfg.UserThemes, cfg.Theme["name"])
			if args.JSON {
				b, err := json.Marshal(struct {
					Themes []ui.ThemeInfo `json:"themes"`
				}{Themes: list})
				if err != nil {
					fail(1, err)
					return
				}
				writeln(deps.out, string(b))
				return
			}
			for _, item := range list {
				mark := " "
				if item.Active {
					mark = "*"
				}
				writef(deps.out, "%s %-18s  %s\n", mark, item.Name, item.Source)
			}
			writef(deps.out, "\nThemes directory: %s\n", config.ThemesDir())
			writef(deps.out, "Export a theme:   slat theme export <name>\n")
		}, catTheme)

	type themeShowArgs struct {
		Name string `cli:"name, theme name to show (defaults to active theme)"`
	}
	addCmd(m, "theme show", "print a theme TOML definition to stdout", 1, checkCmd, themeParseOpts,
		func(_ *mcli.Context, args *themeShowArgs) {
			data, _, err := resolveThemeData(args.Name)
			if err != nil {
				fail(1, err)
				return
			}
			_, _ = deps.out.Write(data)
		}, catTheme)

	type themeExportArgs struct {
		Name  string `cli:"name, theme name to export (defaults to active theme)"`
		Out   string `cli:"-o, --out, output file path (default <name>.toml, or - for stdout)"`
		Force bool   `cli:"-f, --force, overwrite existing output file"`
	}
	addCmd(m, "theme export", "dump a theme TOML file for customization", 1, checkCmd, themeParseOpts,
		func(_ *mcli.Context, args *themeExportArgs) {
			data, canonicalName, err := resolveThemeData(args.Name)
			if err != nil {
				fail(1, err)
				return
			}
			if args.Out == "-" {
				_, _ = deps.out.Write(data)
				return
			}
			outPath := strings.TrimSpace(args.Out)
			if outPath == "" {
				outPath = canonicalName + ".toml"
			}
			if !args.Force {
				if _, statErr := os.Stat(outPath); statErr == nil {
					fail(1, fmt.Errorf("%s already exists (use --force to overwrite)", outPath))
					return
				}
			}
			if parent := filepath.Dir(outPath); parent != "." && parent != "" {
				if mkErr := os.MkdirAll(parent, 0o755); mkErr != nil {
					fail(1, mkErr)
					return
				}
			}
			if writeErr := os.WriteFile(outPath, data, 0o644); writeErr != nil {
				fail(1, writeErr)
				return
			}
			dest := filepath.Join(config.ThemesDir(), canonicalName+".toml")
			writef(deps.out, "Exported theme %q to %s\n", canonicalName, outPath)
			if absOut, absErr := filepath.Abs(outPath); absErr != nil || absOut != dest {
				writef(deps.out, "Place it at %s to customize or override it,\n", dest)
				writef(deps.out, "then set `name = %q` under [theme] in %s.\n", canonicalName, config.Path())
			}
		}, catTheme)
}

func completeThemeName(rawPrefix, cfgPath string) []mcli.CompletionItem {
	userThemes, _ := ui.LoadThemesFromDir(config.ThemesDir())
	var activeName string
	if cfg, err := config.LoadFrom(cfgPath); err == nil {
		userThemes = cfg.UserThemes
		activeName = cfg.Theme["name"]
	}
	prefix := strings.ToLower(strings.TrimSpace(rawPrefix))
	var items []mcli.CompletionItem
	for _, info := range ui.ListThemes(userThemes, activeName) {
		if prefix != "" && !strings.HasPrefix(strings.ToLower(info.Name), prefix) {
			continue
		}
		desc := info.Source
		if info.Active {
			desc += " (active)"
		}
		items = append(items, mcli.CompletionItem{
			Value:       info.Name,
			Description: desc,
		})
	}
	return items
}
