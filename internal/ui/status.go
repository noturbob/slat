package ui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/vt"
)

// StatusFormat is the status bar's layout. Each part is a format string of
// literal text and {placeholders}; the placeholders carry the style that
// suits them (the workspace in the accent colour, a tab in the tab colour,
// the active tab highlighted), so a bar can be rearranged without writing
// colour markup by hand.
type StatusFormat struct {
	Left  string // from the left edge: the workspace and the tabs
	Right string // right-aligned: the badge, pane counts, a clock
	Tab   string // repeated for each tab by {tabs}
	Alert string // appended to a tab holding a pane that wants input
}

// DefaultStatusFormat is the bar slat has always drawn.
var DefaultStatusFormat = StatusFormat{
	Left:  " {workspace} │ {tabs}",
	Right: "{badge}{workspaces} pane {pane}/{panes} ",
	Tab:   " {index}:{name}{alert} ",
	Alert: " ?",
}

// statusFields are the placeholders, and what each expands to. Anything
// not listed is a config error rather than a silently empty string.
var statusFields = map[string]func(Status) string{
	"workspace":       func(s Status) string { return s.Workspace },
	"workspace_index": func(s Status) string { return fmt.Sprint(s.WorkspaceIndex + 1) },
	"workspace_count": func(s Status) string { return fmt.Sprint(s.WorkspaceCount) },
	"pane":            func(s Status) string { return fmt.Sprint(s.PaneIndex + 1) },
	"panes":           func(s Status) string { return fmt.Sprint(s.PaneCount) },
	"tab":             func(s Status) string { return fmt.Sprint(s.ActiveTab + 1) },
	"tabs_count":      func(s Status) string { return fmt.Sprint(len(s.Tabs)) },
	"time":            func(s Status) string { return time.Now().Format("15:04") },
	"date":            func(s Status) string { return time.Now().Format("2006-01-02") },
	"seconds":         func(s Status) string { return time.Now().Format("15:04:05") },
	// Handled while drawing, because they bring their own styles:
	"tabs":  nil,
	"badge": nil,
	// Only when more than one workspace exists, so a single-workspace
	// session isn't cluttered by a counter that never changes.
	"workspaces": nil,
}

// StatusFieldNames lists the placeholders, for errors and docs.
func StatusFieldNames() []string {
	names := make([]string, 0, len(statusFields))
	for n := range statusFields {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseStatusFormat reads the [status] table, checking every placeholder so
// a typo is reported when slat starts rather than drawn as literal text.
// Each part takes its own placeholders: the sides take the session's
// values, a tab takes only its own.
func ParseStatusFormat(settings map[string]string) (StatusFormat, error) {
	f := DefaultStatusFormat
	sides := StatusFieldNames()
	tabFields := []string{"alert", "index", "name"}
	parts := map[string]struct {
		into    *string
		allowed []string
	}{
		"left":  {&f.Left, sides},
		"right": {&f.Right, sides},
		"tab":   {&f.Tab, tabFields},
		"alert": {&f.Alert, nil}, // literal text only
	}
	for key, value := range settings {
		part, ok := parts[key]
		if !ok {
			return f, fmt.Errorf("unknown setting %q in [status]", key)
		}
		if err := checkPlaceholders(value, part.allowed); err != nil {
			return f, fmt.Errorf("status.%s: %w", key, err)
		}
		*part.into = value
	}
	return f, nil
}

func checkPlaceholders(format string, allowed []string) error {
	for _, part := range splitFormat(format) {
		if !part.isField {
			continue
		}
		if slices.Contains(allowed, part.text) {
			continue
		}
		if len(allowed) == 0 {
			return fmt.Errorf("{%s}: this one takes plain text, no placeholders", part.text)
		}
		return fmt.Errorf("unknown placeholder {%s} (have %s)",
			part.text, strings.Join(allowed, ", "))
	}
	return nil
}

type formatPart struct {
	text    string
	isField bool
}

// splitFormat breaks a format string into literal runs and {placeholders}.
// An unterminated { is literal text, which is friendlier than an error for
// someone whose bar contains a stray brace.
func splitFormat(format string) []formatPart {
	var parts []formatPart
	for len(format) > 0 {
		open := strings.IndexByte(format, '{')
		if open < 0 {
			return append(parts, formatPart{text: format})
		}
		close := strings.IndexByte(format[open:], '}')
		if close < 0 {
			return append(parts, formatPart{text: format})
		}
		if open > 0 {
			parts = append(parts, formatPart{text: format[:open]})
		}
		parts = append(parts, formatPart{text: format[open+1 : open+close], isField: true})
		format = format[open+close+1:]
	}
	return parts
}

// renderStatus lays out one side of the bar as styled segments. tabs and
// badge expand to several segments of their own.
func renderStatus(format string, st Status, f StatusFormat) []segment {
	var out []segment
	for _, part := range splitFormat(format) {
		if !part.isField {
			out = append(out, segment{text: part.text, style: styleBar})
			continue
		}
		switch part.text {
		case "tabs":
			out = append(out, tabSegments(st, f)...)
		case "badge":
			if st.Badge != "" {
				out = append(out, segment{text: " " + st.Badge + " ", style: styleBadge})
			}
		case "workspaces":
			if st.WorkspaceCount > 1 {
				out = append(out, segment{
					text:  fmt.Sprintf(" ws %d/%d ·", st.WorkspaceIndex+1, st.WorkspaceCount),
					style: styleDim,
				})
			}
		case "workspace":
			out = append(out, segment{text: st.Workspace, style: styleWorkspace})
		default:
			if expand := statusFields[part.text]; expand != nil {
				out = append(out, segment{text: expand(st), style: styleDim})
			}
		}
	}
	return out
}

func tabSegments(st Status, f StatusFormat) []segment {
	var out []segment
	for i, name := range st.Tabs {
		style := styleTab
		if i == st.ActiveTab {
			style = styleTabActive
		}
		alert := ""
		if i < len(st.TabAlert) && st.TabAlert[i] {
			alert = f.Alert
		}
		text := strings.NewReplacer(
			"{index}", fmt.Sprint(i+1),
			"{name}", name,
			"{alert}", alert,
		).Replace(f.Tab)
		out = append(out, segment{text: text, style: style})
		out = append(out, segment{text: " ", style: styleBar})
	}
	return out
}

// segment is a run of text with one style.
type segment struct {
	text  string
	style vt.Style
}

func width(segments []segment) int {
	n := 0
	for _, s := range segments {
		n += vt.StringWidth(s.text)
	}
	return n
}
