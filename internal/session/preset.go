package session

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
)

// Preset is a preconfigured slat session stored as a TOML file.
type Preset struct {
	SessionName    string            `toml:"session_name"`
	StartDirectory string            `toml:"start_directory,omitempty"`
	Tabs           []PresetTab       `toml:"tabs,omitempty"`
	Workspaces     []PresetWorkspace `toml:"workspaces,omitempty"`
}

// PresetWorkspace groups tabs under a named workspace in a preset.
type PresetWorkspace struct {
	Name string      `toml:"name"`
	Tabs []PresetTab `toml:"tabs"`
}

// PresetTab defines one tab, its layout mode, and its panes.
type PresetTab struct {
	Name           string       `toml:"name"`
	Layout         string       `toml:"layout,omitempty"`
	StartDirectory string       `toml:"start_directory,omitempty"`
	Panes          []PresetPane `toml:"panes,omitempty"`
}

// PresetPane defines a pane's startup command and optional working directory.
// In TOML it can be written as a command string or a { cmd, cwd } table.
type PresetPane struct {
	Cmd string `toml:"cmd,omitempty"`
	Cwd string `toml:"cwd,omitempty"`
}

// UnmarshalTOML decodes a pane from either a command string or a table.
func (p *PresetPane) UnmarshalTOML(v any) error {
	switch val := v.(type) {
	case string:
		p.Cmd = val
		return nil
	case map[string]any:
		if c, ok := val["cmd"].(string); ok {
			p.Cmd = c
		}
		if d, ok := val["cwd"].(string); ok {
			p.Cwd = d
		}
		return nil
	default:
		return fmt.Errorf("pane must be a command string or {cmd, cwd} table")
	}
}

// SessionsDir returns the directory where preconfigured session TOML files live,
// honoring $XDG_CONFIG_HOME (defaulting to ~/.config/slat/sessions).
func SessionsDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "slat", "sessions"), nil
}

// ExpandPath expands a leading "~/" or "~" to the user's home directory.
func ExpandPath(p string) string {
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// ResolvePresetPath resolves either a file path or a session name inside
// ~/.config/slat/sessions/<name>.toml.
func ResolvePresetPath(nameOrPath string) (string, error) {
	expanded := ExpandPath(strings.TrimSpace(nameOrPath))
	if expanded == "" {
		return "", fmt.Errorf("session name or path cannot be empty")
	}
	if fi, err := os.Stat(expanded); err == nil && !fi.IsDir() {
		return expanded, nil
	}
	if strings.ContainsRune(expanded, '/') || strings.ContainsRune(expanded, filepath.Separator) || strings.HasSuffix(expanded, ".toml") {
		return expanded, nil
	}
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, expanded+".toml"), nil
}

// PresetPathForName returns the path for a named session in SessionsDir.
func PresetPathForName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	clean = strings.TrimSuffix(clean, ".toml")
	if clean == "" || strings.ContainsAny(clean, `/\`) {
		return "", fmt.Errorf("invalid session name %q", name)
	}
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, clean+".toml"), nil
}

// LoadPreset reads and validates a preconfigured session TOML file.
func LoadPreset(path string) (Preset, error) {
	var p Preset
	md, err := toml.DecodeFile(path, &p)
	if err != nil {
		return Preset{}, err
	}
	if len(md.Undecoded()) > 0 {
		return Preset{}, fmt.Errorf("%s: unknown setting %q", path, md.Undecoded()[0].String())
	}
	for _, t := range p.allTabs() {
		if t.Layout != "" {
			if _, ok := layout.ParseMode(t.Layout); !ok {
				return Preset{}, fmt.Errorf("%s: unknown layout %q in tab %q", path, t.Layout, t.Name)
			}
		}
	}
	return p, nil
}

func (p Preset) allTabs() []PresetTab {
	var out []PresetTab
	out = append(out, p.Tabs...)
	for _, ws := range p.Workspaces {
		out = append(out, ws.Tabs...)
	}
	return out
}

// SavePreset writes a Preset atomically to path.
func SavePreset(path string, p Preset) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(p); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".preset-*.toml")
	if err != nil {
		return err
	}
	defer func() {
		if rErr := os.Remove(tmp.Name()); rErr != nil && !os.IsNotExist(rErr) {
			fmt.Fprintf(os.Stderr, "slat: remove temp preset %s: %v\n", tmp.Name(), rErr)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		if cErr := tmp.Close(); cErr != nil {
			return fmt.Errorf("%w (close: %v)", err, cErr)
		}
		return err
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		if cErr := tmp.Close(); cErr != nil {
			return fmt.Errorf("%w (close: %v)", err, cErr)
		}
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ExportPreset captures the current session into a Preset struct.
func (m *Manager) ExportPreset() Preset {
	p := Preset{
		SessionName:    m.SessionName,
		StartDirectory: m.activeDir(),
	}
	toPresetTab := func(tab *Tab) PresetTab {
		mode := tab.LayoutMode
		if mode == "" {
			if tab.Stacked {
				mode = layout.ModeStack
			} else {
				mode = layout.ModeEvenVertical
			}
		}
		pt := PresetTab{
			Name:   tab.Name,
			Layout: string(mode),
		}
		for _, pn := range layout.CollectPanes(tab.Layout) {
			pp := PresetPane{Cwd: pn.Cwd()}
			if _, fg, _ := pn.Foreground(); fg != "" && fg != filepath.Base(m.shell) {
				pp.Cmd = fg
			}
			pt.Panes = append(pt.Panes, pp)
		}
		return pt
	}

	if len(m.Workspaces) == 1 && m.Workspaces[0].Name == "main" {
		for _, tab := range m.Workspaces[0].Tabs {
			p.Tabs = append(p.Tabs, toPresetTab(tab))
		}
		return p
	}
	for _, ws := range m.Workspaces {
		pw := PresetWorkspace{Name: ws.Name}
		for _, tab := range ws.Tabs {
			pw.Tabs = append(pw.Tabs, toPresetTab(tab))
		}
		p.Workspaces = append(p.Workspaces, pw)
	}
	return p
}

// ApplyPreset initializes the session from a preconfigured Preset.
func (m *Manager) ApplyPreset(p Preset, area layout.Rect) error {
	m.Workspaces = nil
	m.nextPane = 0
	m.nextWS = 0
	if p.SessionName != "" {
		m.SessionName = p.SessionName
	}
	baseDir := ExpandPath(p.StartDirectory)

	workspaces := p.Workspaces
	if len(workspaces) == 0 {
		tabs := p.Tabs
		if len(tabs) == 0 {
			tabs = []PresetTab{{Name: "shell"}}
		}
		workspaces = []PresetWorkspace{{Name: "main", Tabs: tabs}}
	}

	for _, pw := range workspaces {
		m.nextWS++
		wsName := pw.Name
		if wsName == "" {
			wsName = fmt.Sprintf("ws-%d", m.nextWS)
		}
		ws := &Workspace{Name: wsName}
		for _, pt := range pw.Tabs {
			tab, err := m.buildPresetTab(pt, baseDir, area)
			if err != nil {
				return err
			}
			ws.Tabs = append(ws.Tabs, tab)
		}
		if len(ws.Tabs) > 0 {
			m.Workspaces = append(m.Workspaces, ws)
		}
	}
	if len(m.Workspaces) == 0 {
		return m.AddWorkspace(area)
	}
	m.ActiveWorkspaceIdx = 0
	return nil
}

func (m *Manager) buildPresetTab(pt PresetTab, baseDir string, area layout.Rect) (*Tab, error) {
	mode := layout.ModeEvenVertical
	if pt.Layout != "" {
		parsed, ok := layout.ParseMode(pt.Layout)
		if !ok {
			return nil, fmt.Errorf("unknown layout %q", pt.Layout)
		}
		mode = parsed
	}
	tabDir := ExpandPath(pt.StartDirectory)
	if tabDir == "" {
		tabDir = baseDir
	}
	specs := pt.Panes
	if len(specs) == 0 {
		specs = []PresetPane{{}}
	}
	panes := make([]*pane.Pane, 0, len(specs))
	for _, sp := range specs {
		dir := ExpandPath(sp.Cwd)
		if dir == "" {
			dir = tabDir
		}
		pn, err := m.newPane(area.Rows, area.Cols, dir)
		if err != nil {
			for _, created := range panes {
				created.Close()
			}
			return nil, err
		}
		if cmd := strings.TrimSpace(sp.Cmd); cmd != "" {
			_, _ = pn.Write([]byte(cmd + "\r"))
		}
		panes = append(panes, pn)
	}
	name := pt.Name
	customName := name != ""
	if name == "" {
		name = "shell"
	}
	return &Tab{
		Name:       name,
		CustomName: customName,
		Stacked:    mode == layout.ModeStack,
		LayoutMode: mode,
		Layout:     layout.Arrange(panes, mode),
		ActivePane: panes[0],
	}, nil
}
