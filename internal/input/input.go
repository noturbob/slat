package input

import (
	"strings"
)

// ParsePrefix converts a prefix string like "C-s" to the actual byte value.
func ParsePrefix(prefix string) byte {
	prefix = strings.TrimSpace(prefix)
	if strings.HasPrefix(prefix, "C-") && len(prefix) == 3 {
		ch := prefix[2]
		// Ctrl+key = key & 0x1f
		if ch >= 'a' && ch <= 'z' {
			return ch & 0x1f
		}
		if ch >= 'A' && ch <= 'Z' {
			return (ch + 32) & 0x1f // normalize to lowercase
		}
	}
	// Fallback: literal single char
	if len(prefix) == 1 {
		return prefix[0]
	}
	// Default: Ctrl-S (0x13)
	return 0x13
}

// Action represents a user command.
type Action int

const (
	ActionNone Action = iota
	ActionForwardInput
	ActionSplitVertical
	ActionSplitHorizontal
	ActionNextPane
	ActionPrevPane
	ActionClosePane
	ActionNewTab
	ActionNextTab
	ActionPrevTab
	ActionNewWorkspace
	ActionNextWorkspace
	ActionZoom
	ActionQuit
	ActionShowHelp
)

// Handler processes input bytes and maps them to actions via a prefix key system.
type Handler struct {
	prefixByte byte
	keybinds   map[string]string
	prefixMode bool
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
	return h.prefixMode
}

// ProcessByte processes a single byte of input and returns an action.
func (h *Handler) ProcessByte(b byte) Action {
	if !h.prefixMode {
		if b == h.prefixByte {
			h.prefixMode = true
			return ActionNone
		}
		return ActionForwardInput
	}

	// We're in prefix mode — interpret the key
	h.prefixMode = false

	key := string(b)

	// Check keybinds map
	for action, binding := range h.keybinds {
		if binding == key {
			switch action {
			case "split-vertical":
				return ActionSplitVertical
			case "split-horizontal":
				return ActionSplitHorizontal
			case "next-pane":
				return ActionNextPane
			case "prev-pane":
				return ActionPrevPane
			case "close-pane":
				return ActionClosePane
			case "new-tab":
				return ActionNewTab
			case "next-tab":
				return ActionNextTab
			case "prev-tab":
				return ActionPrevTab
			case "new-workspace":
				return ActionNewWorkspace
			case "next-workspace":
				return ActionNextWorkspace
			case "zoom":
				return ActionZoom
			case "quit":
				return ActionQuit
			case "detach":
				return ActionQuit // for now, detach == quit
			}
		}
	}

	// '?' for help
	if key == "?" {
		return ActionShowHelp
	}

	// Send the prefix key itself if pressed twice
	if b == h.prefixByte {
		return ActionForwardInput
	}

	return ActionNone
}
