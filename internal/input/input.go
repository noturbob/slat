package input

import (
	"fmt"
	"strings"
)

// ParsePrefix converts a prefix string like "C-s" to the byte the terminal
// sends for it. It returns an error for anything it can't represent.
func ParsePrefix(prefix string) (byte, error) {
	s := strings.TrimSpace(prefix)
	if len(s) == 3 && (s[:2] == "C-" || s[:2] == "c-") {
		ch := s[2]
		switch {
		case ch >= 'a' && ch <= 'z':
			return ch & 0x1f, nil
		case ch >= 'A' && ch <= 'Z':
			return (ch + 32) & 0x1f, nil
		case ch == '[':
			return 0, fmt.Errorf("prefix %q is the Escape key, which would break arrow and function keys", prefix)
		case ch >= '@' && ch <= '_': // C-@ C-\ C-] C-^ C-_
			return ch & 0x1f, nil
		}
	}
	if len(s) == 1 {
		return s[0], nil
	}
	return 0, fmt.Errorf("invalid prefix %q (use e.g. \"C-a\" or \"C-s\")", prefix)
}

// PrefixName renders a prefix byte the way users write it, e.g. "Ctrl-S".
func PrefixName(b byte) string {
	if b < 0x20 {
		return "Ctrl-" + string(b|0x40)
	}
	return string(b)
}

// Action represents a user command.
type Action int

const (
	ActionNone Action = iota
	ActionForwardInput
	// Pane management
	ActionSplitVertical
	ActionSplitHorizontal
	ActionNextPane
	ActionPrevPane
	ActionClosePane
	ActionZoom
	ActionSelectPaneUp
	ActionSelectPaneDown
	ActionSelectPaneLeft
	ActionSelectPaneRight
	ActionSwapPane
	ActionResizeGrow
	ActionResizeShrink
	ActionEqualizeLayout
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
	ActionQuit
	ActionDetach
	ActionSendPrefix
	ActionScrollMode
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
	{"next-pane", "Panes", "Next pane", ActionNextPane},
	{"prev-pane", "Panes", "Previous pane", ActionPrevPane},
	{"select-pane-up", "Panes", "Pane above (or ↑)", ActionSelectPaneUp},
	{"select-pane-down", "Panes", "Pane below (or ↓)", ActionSelectPaneDown},
	{"select-pane-left", "Panes", "Pane left (or ←)", ActionSelectPaneLeft},
	{"select-pane-right", "Panes", "Pane right (or →)", ActionSelectPaneRight},
	{"swap-pane", "Panes", "Swap with next pane", ActionSwapPane},
	{"resize-grow", "Panes", "Grow pane", ActionResizeGrow},
	{"resize-shrink", "Panes", "Shrink pane", ActionResizeShrink},
	{"equalize", "Panes", "Equalize sizes", ActionEqualizeLayout},
	{"zoom", "Panes", "Toggle zoom", ActionZoom},
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
	{"detach", "Session", "Detach (keeps running)", ActionDetach},
	{"quit", "Session", "Quit (ends session)", ActionQuit},
}

// Handler maps input bytes to actions via a prefix key.
// It is not safe for concurrent use.
type Handler struct {
	prefix     byte
	keys       map[byte]Action
	prefixMode bool
}

// NewHandler creates an input handler. keybinds maps binding names to
// single-character keys; it must already be validated (see config.Load).
func NewHandler(prefix byte, keybinds map[string]string) *Handler {
	h := &Handler{prefix: prefix, keys: map[byte]Action{}}
	for _, b := range Bindings {
		if k := keybinds[b.Name]; len(k) == 1 {
			h.keys[k[0]] = b.Action
		}
	}
	return h
}

// IsPrefixActive reports whether the next key will be read as a command.
func (h *Handler) IsPrefixActive() bool { return h.prefixMode }

// PrefixByte returns the raw prefix byte.
func (h *Handler) PrefixByte() byte { return h.prefix }

// CancelPrefix leaves prefix mode without running a command.
func (h *Handler) CancelPrefix() { h.prefixMode = false }

// ProcessByte processes a single byte of input and returns an action.
func (h *Handler) ProcessByte(b byte) Action {
	if !h.prefixMode {
		if b == h.prefix {
			h.prefixMode = true
			return ActionNone
		}
		return ActionForwardInput
	}
	h.prefixMode = false

	if b == h.prefix {
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
