package input

import (
	"strings"
	"sync/atomic"
)

// ParsePrefix converts a prefix string like "C-s" to the actual byte value.
func ParsePrefix(prefix string) byte {
	prefix = strings.TrimSpace(prefix)
	if strings.HasPrefix(prefix, "C-") && len(prefix) == 3 {
		ch := prefix[2]
		if ch >= 'a' && ch <= 'z' {
			return ch & 0x1f
		}
		if ch >= 'A' && ch <= 'Z' {
			return (ch + 32) & 0x1f
		}
	}
	if len(prefix) == 1 {
		return prefix[0]
	}
	return 0x13 // Ctrl-S
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
	ActionDetach // BUG FIX: this used to not exist — "detach" was wrongly aliased to ActionQuit
	ActionSendPrefix
)

// Handler processes input bytes and maps them to actions via a prefix key system.
type Handler struct {
	prefixByte byte
	keybinds   map[string]string
	// BUG FIX: prefixMode used to be a plain bool, read from one goroutine
	// (readInput) and written from another (status bar render loop reads it
	// via IsPrefixActive concurrently). That's a data race. atomic.Bool fixes it.
	prefixMode atomic.Bool
}

// NewHandler creates an input handler with the given prefix key and keybind map.
func NewHandler(prefix string, keybinds map[string]string) *Handler {
	return &Handler{
		prefixByte: ParsePrefix(prefix),
		keybinds:   keybinds,
	}
}

// IsPrefixActive returns whether we're currently waiting for a command key.
func (h *Handler) IsPrefixActive() bool {
	return h.prefixMode.Load()
}

// PrefixByte exposes the raw prefix byte (used by the input fast-path).
func (h *Handler) PrefixByte() byte {
	return h.prefixByte
}

// actionMap maps config keybind names to Action constants.
var actionMap = map[string]Action{
	"split-vertical":    ActionSplitVertical,
	"split-horizontal":  ActionSplitHorizontal,
	"next-pane":         ActionNextPane,
	"prev-pane":         ActionPrevPane,
	"close-pane":        ActionClosePane,
	"zoom":              ActionZoom,
	"select-pane-up":    ActionSelectPaneUp,
	"select-pane-down":  ActionSelectPaneDown,
	"select-pane-left":  ActionSelectPaneLeft,
	"select-pane-right": ActionSelectPaneRight,
	"swap-pane":         ActionSwapPane,
	"resize-grow":       ActionResizeGrow,
	"resize-shrink":     ActionResizeShrink,
	"equalize":          ActionEqualizeLayout,
	"new-tab":           ActionNewTab,
	"next-tab":          ActionNextTab,
	"prev-tab":          ActionPrevTab,
	"close-tab":         ActionCloseTab,
	"rename-tab":        ActionRenameTab,
	"new-workspace":     ActionNewWorkspace,
	"next-workspace":    ActionNextWorkspace,
	"prev-workspace":    ActionPrevWorkspace,
	"rename-workspace":  ActionRenameWorkspace,
	"quit":              ActionQuit,
	"detach":            ActionDetach, // was: ActionQuit — this was the bug
}

// ProcessByte processes a single byte of input and returns an action.
func (h *Handler) ProcessByte(b byte) Action {
	if !h.prefixMode.Load() {
		if b == h.prefixByte {
			h.prefixMode.Store(true)
			return ActionNone
		}
		return ActionForwardInput
	}
	// We're in prefix mode — interpret the key
	h.prefixMode.Store(false)

	// Send the prefix key itself if pressed twice
	if b == h.prefixByte {
		return ActionSendPrefix
	}

	key := string(b)

	// Number keys 1-9 for direct tab access
	if b >= '1' && b <= '9' {
		for action, binding := range h.keybinds {
			if binding == key {
				if a, ok := actionMap[action]; ok {
					return a
				}
			}
		}
		return ActionGoToTab1 + Action(b-'1')
	}

	// Check keybinds map
	for action, binding := range h.keybinds {
		if binding == key {
			if a, ok := actionMap[action]; ok {
				return a
			}
		}
	}

	// Built-in keys not in the config
	if key == "?" {
		return ActionShowHelp
	}
	return ActionNone
}