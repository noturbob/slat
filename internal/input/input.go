package input

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Prefix represents a parsed prefix key, which may be a single byte
// (e.g. Ctrl-S -> 0x13) or a multi-byte terminal escape sequence (e.g. F13 -> \x1b[25~).
type Prefix struct {
	Name    string   // human-readable name, e.g. "Ctrl-S", "F13", `\e[25~`
	Bytes   []byte   // canonical byte sequence forwarded by ActionSendPrefix
	Aliases [][]byte // alternative sequences recognized as this prefix
}

var functionKeys = map[string]Prefix{
	"F13": {Name: "F13", Bytes: []byte("\x1b[25~"), Aliases: [][]byte{[]byte("\x1b[25;1~"), []byte("\x1b[57376u")}},
	"F14": {Name: "F14", Bytes: []byte("\x1b[26~"), Aliases: [][]byte{[]byte("\x1b[26;1~"), []byte("\x1b[57377u")}},
	"F15": {Name: "F15", Bytes: []byte("\x1b[28~"), Aliases: [][]byte{[]byte("\x1b[28;1~"), []byte("\x1b[57378u")}},
	"F16": {Name: "F16", Bytes: []byte("\x1b[29~"), Aliases: [][]byte{[]byte("\x1b[29;1~"), []byte("\x1b[57379u")}},
	"F17": {Name: "F17", Bytes: []byte("\x1b[31~"), Aliases: [][]byte{[]byte("\x1b[31;1~"), []byte("\x1b[57380u")}},
	"F18": {Name: "F18", Bytes: []byte("\x1b[32~"), Aliases: [][]byte{[]byte("\x1b[32;1~"), []byte("\x1b[57381u")}},
	"F19": {Name: "F19", Bytes: []byte("\x1b[33~"), Aliases: [][]byte{[]byte("\x1b[33;1~"), []byte("\x1b[57382u")}},
	"F20": {Name: "F20", Bytes: []byte("\x1b[34~"), Aliases: [][]byte{[]byte("\x1b[34;1~"), []byte("\x1b[57383u")}},
	"F21": {Name: "F21", Bytes: []byte("\x1b[20;2~"), Aliases: [][]byte{[]byte("\x1b[35~"), []byte("\x1b[35;1~"), []byte("\x1b[57384u")}},
	"F22": {Name: "F22", Bytes: []byte("\x1b[21;2~"), Aliases: [][]byte{[]byte("\x1b[36~"), []byte("\x1b[36;1~"), []byte("\x1b[57385u")}},
	"F23": {Name: "F23", Bytes: []byte("\x1b[23;2~"), Aliases: [][]byte{[]byte("\x1b[37~"), []byte("\x1b[37;1~"), []byte("\x1b[57386u")}},
	"F24": {Name: "F24", Bytes: []byte("\x1b[24;2~"), Aliases: [][]byte{[]byte("\x1b[38~"), []byte("\x1b[38;1~"), []byte("\x1b[57387u")}},
}

// ParsePrefix converts a prefix string like "C-s", "F13", or `\e[25~` to a
// Prefix. It returns an error for anything it can't represent.
func ParsePrefix(prefix string) (Prefix, error) {
	s := strings.TrimSpace(prefix)
	if len(s) == 3 && (s[:2] == "C-" || s[:2] == "c-") {
		ch := s[2]
		switch {
		case ch >= 'a' && ch <= 'z':
			b := ch & 0x1f
			return Prefix{Name: singleByteName(b), Bytes: []byte{b}}, nil
		case ch >= 'A' && ch <= 'Z':
			b := (ch + 32) & 0x1f
			return Prefix{Name: singleByteName(b), Bytes: []byte{b}}, nil
		case ch == '[':
			return Prefix{}, fmt.Errorf("prefix %q is the Escape key, which would break arrow and function keys", prefix)
		case ch >= '@' && ch <= '_': // C-@ C-\ C-] C-^ C-_
			b := ch & 0x1f
			return Prefix{Name: singleByteName(b), Bytes: []byte{b}}, nil
		}
	}
	if fk, ok := functionKeys[strings.ToUpper(s)]; ok {
		return fk, nil
	}
	if strings.HasPrefix(s, `\`) || strings.HasPrefix(s, "\x1b") {
		seq, err := unescapePrefix(s)
		if err != nil {
			return Prefix{}, fmt.Errorf("invalid prefix %q: %w", prefix, err)
		}
		if len(seq) == 1 && seq[0] == 0x1b {
			return Prefix{}, fmt.Errorf("prefix %q is the Escape key, which would break arrow and function keys", prefix)
		}
		if len(seq) >= 2 && seq[0] == 0x1b {
			return Prefix{Name: formatSequenceName(seq), Bytes: seq}, nil
		}
	}
	if len(s) == 1 {
		b := s[0]
		if b == 0x1b {
			return Prefix{}, fmt.Errorf("prefix %q is the Escape key, which would break arrow and function keys", prefix)
		}
		return Prefix{Name: singleByteName(b), Bytes: []byte{b}}, nil
	}
	return Prefix{}, fmt.Errorf("invalid prefix %q (use e.g. \"C-a\", \"C-s\", \"F13\", or `\\e[25~`)", prefix)
}

func singleByteName(b byte) string {
	if b < 0x20 {
		return "Ctrl-" + string(b|0x40)
	}
	return string(b)
}

func unescapePrefix(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out = append(out, s[i])
			continue
		}
		if i+1 >= len(s) {
			return nil, errors.New("trailing backslash")
		}
		switch s[i+1] {
		case 'e', 'E':
			out = append(out, 0x1b)
			i++
		case '\\':
			out = append(out, '\\')
			i++
		case 'x', 'X':
			if i+3 >= len(s) {
				return nil, errors.New("incomplete hex escape")
			}
			v, err := strconv.ParseUint(s[i+2:i+4], 16, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid hex escape %q", s[i:i+4])
			}
			out = append(out, byte(v))
			i += 3
		default:
			return nil, fmt.Errorf("unsupported escape \\%c", s[i+1])
		}
	}
	return out, nil
}

func formatSequenceName(seq []byte) string {
	var b strings.Builder
	for _, c := range seq {
		switch {
		case c == 0x1b:
			b.WriteString(`\e`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Action represents a user command.
type Action int

const (
	ActionNone Action = iota
	ActionForwardInput
	// Pane management
	ActionSplitVertical
	ActionSplitHorizontal
	ActionAddPane
	ActionNextPane
	ActionPrevPane
	ActionClosePane
	ActionZoom
	ActionIdentifyPanes
	ActionStackMode
	ActionSelectPaneUp
	ActionSelectPaneDown
	ActionSelectPaneLeft
	ActionSelectPaneRight
	ActionSwapPane
	ActionMovePaneUp
	ActionMovePaneDown
	ActionMovePaneLeft
	ActionMovePaneRight
	ActionMoveMode
	ActionResizeGrow
	ActionResizeShrink
	ActionEqualizeLayout
	ActionLayoutPicker
	ActionBreakPane
	ActionJoinPane
	// Tab management
	ActionNewTab
	ActionNextTab
	ActionPrevTab
	ActionCloseTab
	ActionRenameTab
	ActionGoToTab1
	ActionGoToTab2
	ActionGoToTab3
	ActionGoToTab4
	ActionGoToTab5
	ActionGoToTab6
	ActionGoToTab7
	ActionGoToTab8
	ActionGoToTab9
	// Workspace management
	ActionNewWorkspace
	ActionNextWorkspace
	ActionPrevWorkspace
	ActionRenameWorkspace
	// App / session
	ActionShowHelp
	ActionRenameSession
	ActionTreePicker
	ActionSessionPicker
	ActionNextSession
	ActionPrevSession
	ActionGlobalTreePicker
	ActionSaveSession
	ActionQuit
	ActionDetach
	ActionSendPrefix
	ActionScrollMode
	ActionToggleStatusBar
)

// Binding describes one configurable command.
type Binding struct {
	Name   string // config key, e.g. "split-vertical"
	Group  string // help section
	Desc   string
	Action Action
}

// Bindings lists every configurable command, in help-overlay order.
var Bindings = []Binding{
	{"split-vertical", "Panes", "Split left / right", ActionSplitVertical},
	{"split-horizontal", "Panes", "Split top / bottom", ActionSplitHorizontal},
	{"add-pane", "Panes", "Add pane (honors layout)", ActionAddPane},
	{"next-pane", "Panes", "Next pane", ActionNextPane},
	{"prev-pane", "Panes", "Previous pane", ActionPrevPane},
	{"select-pane-up", "Panes", "Pane above (or ↑)", ActionSelectPaneUp},
	{"select-pane-down", "Panes", "Pane below (or ↓)", ActionSelectPaneDown},
	{"select-pane-left", "Panes", "Pane left (or ←)", ActionSelectPaneLeft},
	{"select-pane-right", "Panes", "Pane right (or →)", ActionSelectPaneRight},
	{"swap-pane", "Panes", "Swap with next pane", ActionSwapPane},
	{"move-mode", "Panes", "Move the pane around (hjkl, q exits)", ActionMoveMode},
	{"move-pane-up", "Panes", "Move pane up", ActionMovePaneUp},
	{"move-pane-down", "Panes", "Move pane down", ActionMovePaneDown},
	{"move-pane-left", "Panes", "Move pane left", ActionMovePaneLeft},
	{"move-pane-right", "Panes", "Move pane right", ActionMovePaneRight},
	{"resize-grow", "Panes", "Grow pane", ActionResizeGrow},
	{"resize-shrink", "Panes", "Shrink pane", ActionResizeShrink},
	{"equalize", "Panes", "Equalize sizes", ActionEqualizeLayout},
	{"layout-picker", "Panes", "Choose layout (fuzzy)", ActionLayoutPicker},
	{"zoom", "Panes", "Toggle zoom", ActionZoom},
	{"identify-panes", "Panes", "Toggle pane ID badges", ActionIdentifyPanes},
	{"stack-mode", "Panes", "Toggle stacked mode", ActionStackMode},
	{"break-pane", "Panes", "Break pane into new tab", ActionBreakPane},
	{"join-pane", "Panes", "Join pane into current tab", ActionJoinPane},
	{"close-pane", "Panes", "Close pane", ActionClosePane},
	{"scroll-mode", "Panes", "Scroll back (q exits)", ActionScrollMode},
	{"new-tab", "Tabs", "New tab", ActionNewTab},
	{"next-tab", "Tabs", "Next tab", ActionNextTab},
	{"prev-tab", "Tabs", "Previous tab", ActionPrevTab},
	{"rename-tab", "Tabs", "Rename tab", ActionRenameTab},
	{"close-tab", "Tabs", "Close tab", ActionCloseTab},
	{"new-workspace", "Workspaces", "New workspace", ActionNewWorkspace},
	{"next-workspace", "Workspaces", "Next workspace", ActionNextWorkspace},
	{"prev-workspace", "Workspaces", "Previous workspace", ActionPrevWorkspace},
	{"rename-workspace", "Workspaces", "Rename workspace", ActionRenameWorkspace},
	{"rename-session", "Session", "Rename session", ActionRenameSession},
	{"tree-picker", "Session", "Fuzzy tree picker", ActionTreePicker},
	{"session-picker", "Session", "Switch session (fuzzy)", ActionSessionPicker},
	{"next-session", "Session", "Next session", ActionNextSession},
	{"prev-session", "Session", "Previous session", ActionPrevSession},
	{"global-tree", "Session", "All sessions/tabs/panes tree", ActionGlobalTreePicker},
	{"save-session", "Session", "Save preconfigured session", ActionSaveSession},
	{"toggle-status-bar", "Session", "Toggle status bar", ActionToggleStatusBar},
	{"detach", "Session", "Detach (keeps running)", ActionDetach},
	{"quit", "Session", "Quit (ends session)", ActionQuit},
}

// Shortcut represents a parsed direct shortcut (triggered without the prefix key).
type Shortcut struct {
	Name    string
	Seqs    [][]byte
	Command string
}

// ParseShortcut converts a shortcut spec like "Alt+1", "M-1", "F13", or
// `\e[25~` into its recognized terminal byte sequences and display name.
func ParseShortcut(spec string) ([][]byte, string, error) {
	s := strings.TrimSpace(spec)
	if s == "" {
		return nil, "", errors.New("empty shortcut")
	}
	lower := strings.ToLower(s)
	var rest string
	switch {
	case strings.HasPrefix(lower, "alt+"):
		rest = s[4:]
	case strings.HasPrefix(lower, "alt-"):
		rest = s[4:]
	case strings.HasPrefix(lower, "m-"):
		rest = s[2:]
	}
	if rest != "" {
		if len(rest) != 1 || rest[0] < 0x20 || rest[0] >= 0x7f {
			return nil, "", fmt.Errorf("invalid Alt shortcut %q (use e.g. \"Alt+1\" or \"M-s\")", spec)
		}
		ch := rest[0]
		seqs := [][]byte{
			{0x1b, ch},
			fmt.Appendf(nil, "\x1b[%d;3u", ch),
		}
		return seqs, "Alt+" + string(ch), nil
	}
	if fk, ok := functionKeys[strings.ToUpper(s)]; ok {
		seqs := make([][]byte, 0, 1+len(fk.Aliases))
		seqs = append(seqs, fk.Bytes)
		seqs = append(seqs, fk.Aliases...)
		return seqs, fk.Name, nil
	}
	if strings.HasPrefix(s, `\`) || strings.HasPrefix(s, "\x1b") {
		seq, err := unescapePrefix(s)
		if err != nil {
			return nil, "", fmt.Errorf("invalid shortcut %q: %w", spec, err)
		}
		if len(seq) < 2 || seq[0] != 0x1b {
			return nil, "", fmt.Errorf("shortcut %q must be a multi-byte escape sequence", spec)
		}
		return [][]byte{seq}, formatSequenceName(seq), nil
	}
	return nil, "", fmt.Errorf("invalid shortcut %q (use e.g. \"Alt+1\", \"M-1\", \"F13\", or `\\e[25~`)", spec)
}

// Handler maps input bytes to actions via a prefix key, plus optional direct shortcuts.
// It is not safe for concurrent use.
type Handler struct {
	prefix        Prefix
	keys          map[byte]Action
	prefixMode    bool
	singleLead    bool
	leadByte      byte
	shortcuts     []Shortcut
	shortcutLeads [256]bool
}

// NewHandler creates an input handler. keybinds maps binding names to
// single-character keys; it must already be validated (see config.Load).
func NewHandler(prefix Prefix, keybinds map[string]string) *Handler {
	h := &Handler{prefix: prefix, keys: map[byte]Action{}}
	if len(prefix.Bytes) > 0 {
		lead := prefix.Bytes[0]
		allSame := true
		for _, a := range prefix.Aliases {
			if len(a) > 0 && a[0] != lead {
				allSame = false
				break
			}
		}
		if allSame {
			h.singleLead = true
			h.leadByte = lead
		}
	}
	for _, b := range Bindings {
		if k := keybinds[b.Name]; len(k) == 1 {
			h.keys[k[0]] = b.Action
		}
	}
	return h
}

// SetShortcuts parses and registers direct (non-prefix) shortcuts.
func (h *Handler) SetShortcuts(shortcuts map[string]string) error {
	h.shortcuts = nil
	h.shortcutLeads = [256]bool{}
	if len(shortcuts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(shortcuts))
	for k := range shortcuts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := make(map[string]string)
	parsed := make([]Shortcut, 0, len(keys))
	for _, k := range keys {
		cmd := strings.TrimSpace(shortcuts[k])
		if cmd == "" {
			return fmt.Errorf("shortcut %q: command must not be empty", k)
		}
		seqs, name, err := ParseShortcut(k)
		if err != nil {
			return err
		}
		for _, seq := range seqs {
			key := string(seq)
			if prev, ok := seen[key]; ok {
				return fmt.Errorf("shortcut %q conflicts with %q", k, prev)
			}
			seen[key] = k
			h.shortcutLeads[seq[0]] = true
		}
		parsed = append(parsed, Shortcut{Name: name, Seqs: seqs, Command: cmd})
	}
	h.shortcuts = parsed
	return nil
}

// MatchShortcut returns the command and byte length if buf begins with a
// registered direct shortcut sequence.
func (h *Handler) MatchShortcut(buf []byte) (string, int) {
	if len(buf) == 0 || !h.shortcutLeads[buf[0]] {
		return "", 0
	}
	for _, sc := range h.shortcuts {
		for _, seq := range sc.Seqs {
			if bytes.HasPrefix(buf, seq) {
				return sc.Command, len(seq)
			}
		}
	}
	return "", 0
}

// IsPrefixActive reports whether the next key will be read as a command.
func (h *Handler) IsPrefixActive() bool { return h.prefixMode }

// PrefixBytes returns the canonical raw prefix byte sequence.
func (h *Handler) PrefixBytes() []byte { return h.prefix.Bytes }

// CancelPrefix leaves prefix mode without running a command.
func (h *Handler) CancelPrefix() { h.prefixMode = false }

// MatchPrefix returns the length of the prefix sequence at the start of buf,
// or 0 if buf does not begin with the configured prefix (or one of its aliases).
func (h *Handler) MatchPrefix(buf []byte) int {
	if len(buf) == 0 || (h.singleLead && buf[0] != h.leadByte) {
		return 0
	}
	if len(h.prefix.Bytes) > 0 && bytes.HasPrefix(buf, h.prefix.Bytes) {
		return len(h.prefix.Bytes)
	}
	for _, alias := range h.prefix.Aliases {
		if len(alias) > 0 && bytes.HasPrefix(buf, alias) {
			return len(alias)
		}
	}
	return 0
}

// ForwardLen returns how many leading bytes of buf can be forwarded
// verbatim before the next prefix or shortcut sequence (at least 1 when len(buf) > 0).
func (h *Handler) ForwardLen(buf []byte) int {
	if len(buf) <= 1 {
		return len(buf)
	}
	if len(h.shortcuts) == 0 && h.singleLead {
		for off := 1; off < len(buf); {
			idx := bytes.IndexByte(buf[off:], h.leadByte)
			if idx < 0 {
				return len(buf)
			}
			pos := off + idx
			if h.MatchPrefix(buf[pos:]) > 0 {
				return pos
			}
			off = pos + 1
		}
		return len(buf)
	}
	for j := 1; j < len(buf); j++ {
		if (!h.singleLead || buf[j] == h.leadByte) && h.MatchPrefix(buf[j:]) > 0 {
			return j
		}
		if _, n := h.MatchShortcut(buf[j:]); n > 0 {
			return j
		}
	}
	return len(buf)
}

// ProcessPrefix handles a matched prefix sequence: entering prefix mode if
// idle, or returning ActionSendPrefix if prefix mode is already active.
func (h *Handler) ProcessPrefix() Action {
	if !h.prefixMode {
		h.prefixMode = true
		return ActionNone
	}
	h.prefixMode = false
	return ActionSendPrefix
}

// ProcessByte processes a single non-prefix byte of input and returns an action.
func (h *Handler) ProcessByte(b byte) Action {
	if !h.prefixMode {
		if len(h.prefix.Bytes) == 1 && b == h.prefix.Bytes[0] {
			h.prefixMode = true
			return ActionNone
		}
		return ActionForwardInput
	}
	h.prefixMode = false

	if len(h.prefix.Bytes) == 1 && b == h.prefix.Bytes[0] {
		return ActionSendPrefix
	}
	if a, ok := h.keys[b]; ok {
		return a
	}
	if b >= '1' && b <= '9' {
		return ActionGoToTab1 + Action(b-'1')
	}
	if b == '?' {
		return ActionShowHelp
	}
	return ActionNone
}
