package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/noturbob/slat/internal/vt"
)

// BorderSet is the eleven glyphs a pane border is drawn from. Every one can
// be replaced, because a border is the most visible thing slat draws and
// people have opinions about it.
type BorderSet struct {
	Vertical    rune
	Horizontal  rune
	TopLeft     rune
	TopRight    rune
	BottomLeft  rune
	BottomRight rune
	TeeRight    rune // ├
	TeeLeft     rune // ┤
	TeeDown     rune // ┬
	TeeUp       rune // ┴
	Cross       rune
}

// borderStyles are the built-in sets. "sharp" is what slat has always
// drawn; "none" leaves the gap between panes blank.
var borderStyles = map[string]BorderSet{
	"sharp":   {'│', '─', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'},
	"rounded": {'│', '─', '╭', '╮', '╰', '╯', '├', '┤', '┬', '┴', '┼'},
	"heavy":   {'┃', '━', '┏', '┓', '┗', '┛', '┣', '┫', '┳', '┻', '╋'},
	"double":  {'║', '═', '╔', '╗', '╚', '╝', '╠', '╣', '╦', '╩', '╬'},
	"dashed":  {'╎', '╌', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼'},
	"none":    {' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '},
}

// BorderStyleNames lists the built-in styles, for errors and docs.
func BorderStyleNames() []string {
	names := make([]string, 0, len(borderStyles))
	for n := range borderStyles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// borderSet is the set in use. Apply replaces it.
var borderSet = borderStyles["sharp"]

// ParseBorders builds a border set from the [borders] table: a "style"
// naming a built-in set, plus any single glyph overriding it.
func ParseBorders(settings map[string]string) (BorderSet, error) {
	b := borderStyles["sharp"]
	if style, ok := settings["style"]; ok {
		preset, known := borderStyles[strings.ToLower(strings.TrimSpace(style))]
		if !known {
			return b, fmt.Errorf("borders.style = %q: no such style (have %s)",
				style, strings.Join(BorderStyleNames(), ", "))
		}
		b = preset
	}
	fields := map[string]*rune{
		"vertical": &b.Vertical, "horizontal": &b.Horizontal,
		"top_left": &b.TopLeft, "top_right": &b.TopRight,
		"bottom_left": &b.BottomLeft, "bottom_right": &b.BottomRight,
		"tee_right": &b.TeeRight, "tee_left": &b.TeeLeft,
		"tee_down": &b.TeeDown, "tee_up": &b.TeeUp, "cross": &b.Cross,
	}
	for key, value := range settings {
		if key == "style" {
			continue
		}
		field, ok := fields[key]
		if !ok {
			return b, fmt.Errorf("unknown setting %q in [borders]", key)
		}
		g, err := oneCell(value)
		if err != nil {
			return b, fmt.Errorf("borders.%s: %w", key, err)
		}
		*field = g
	}
	return b, nil
}

// Apply makes b the border set everything drawn from now on uses.
func (b BorderSet) Apply() { borderSet = b }

// glyph returns the character for a border cell, given which of its
// neighbours are also border cells.
func (b BorderSet) glyph(mask int) rune {
	switch mask {
	case up, down, up | down:
		return b.Vertical
	case left, right, left | right:
		return b.Horizontal
	case down | right:
		return b.TopLeft
	case down | left:
		return b.TopRight
	case up | right:
		return b.BottomLeft
	case up | left:
		return b.BottomRight
	case up | down | right:
		return b.TeeRight
	case up | down | left:
		return b.TeeLeft
	case left | right | down:
		return b.TeeDown
	case left | right | up:
		return b.TeeUp
	case up | down | left | right:
		return b.Cross
	}
	return b.Vertical
}

// oneCell reads a single-column character. A border cell is one column
// wide, so an emoji or a two-column glyph would tear the layout.
func oneCell(s string) (rune, error) {
	runes := []rune(s)
	if len(runes) != 1 {
		return 0, fmt.Errorf("%q: must be exactly one character", s)
	}
	if w := vt.RuneWidth(runes[0]); w != 1 {
		return 0, fmt.Errorf("%q: must be one column wide, not %d", s, w)
	}
	return runes[0], nil
}
