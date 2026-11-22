package layout

import (
	"math"
	"strings"

	"github.com/noturbob/slat/internal/pane"
)

// Mode identifies a named pane arrangement for a tab.
type Mode string

const (
	ModeEvenHorizontal Mode = "even-horizontal"
	ModeEvenVertical   Mode = "even-vertical"
	ModeMasterLeft     Mode = "master-left"
	ModeMasterRight    Mode = "master-right"
	ModeMasterTop      Mode = "master-top"
	ModeMasterBottom   Mode = "master-bottom"
	ModeGrid           Mode = "grid"
	ModeStack          Mode = "stack"
)

// ModeInfo describes a layout mode for pickers and documentation.
type ModeInfo struct {
	Mode        Mode
	Description string
}

var allModes = []ModeInfo{
	{ModeMasterLeft, "master on the left, stack on the right"},
	{ModeMasterRight, "master on the right, stack on the left"},
	{ModeMasterTop, "master on the top, stack on the bottom"},
	{ModeMasterBottom, "master on the bottom, stack on the top"},
	{ModeEvenHorizontal, "even horizontal (equal columns)"},
	{ModeEvenVertical, "even vertical (equal rows)"},
	{ModeGrid, "grid (every pane equal)"},
	{ModeStack, "stack (active pane full-screen with headers)"},
}

// AllModes returns all supported layout modes in display order.
func AllModes() []ModeInfo {
	out := make([]ModeInfo, len(allModes))
	copy(out, allModes)
	return out
}

// ParseMode normalizes and validates a layout mode name.
func ParseMode(s string) (Mode, bool) {
	norm := strings.ToLower(strings.TrimSpace(s))
	norm = strings.ReplaceAll(norm, "_", "-")
	norm = strings.ReplaceAll(norm, " ", "-")
	for _, m := range allModes {
		if string(m.Mode) == norm {
			return m.Mode, true
		}
	}
	return "", false
}

// Rect is a rectangular region of the screen, 0-based.
type Rect struct {
	Row  int
	Col  int
	Rows int
	Cols int
}

// Contains reports whether the cell (row, col) lies inside r.
func (r Rect) Contains(row, col int) bool {
	return row >= r.Row && row < r.Row+r.Rows && col >= r.Col && col < r.Col+r.Cols
}

// Node is a binary tree node for tiling layout.
// Leaf nodes hold a Pane; internal nodes have two children and a split direction.
type Node struct {
	Split    pane.SplitDirection
	Ratio    float64 // 0.0-1.0, proportion of first child
	Pane     *pane.Pane
	Children [2]*Node
}

// NewLeaf creates a leaf node wrapping a single pane.
func NewLeaf(p *pane.Pane) *Node {
	return &Node{Split: pane.SplitNone, Pane: p, Ratio: 0.5}
}

// SplitNode splits a leaf node into two children in the given direction.
// The existing pane goes to Children[0], the new pane to Children[1].
func SplitNode(n *Node, dir pane.SplitDirection, newPane *pane.Pane) {
	oldPane := n.Pane
	n.Pane = nil
	n.Split = dir
	n.Ratio = 0.5
	n.Children[0] = NewLeaf(oldPane)
	n.Children[1] = NewLeaf(newPane)
}

// splitSizes divides total cells between two children and a 1-cell border.
// Each side gets at least 1 cell whenever total leaves room for it.
func splitSizes(total int, ratio float64) (int, int) {
	first := int(float64(total) * ratio)
	first = min(max(first, 1), total-2)
	first = max(first, 1)
	second := max(total-first-1, 0)
	return first, second
}

// Split returns the rects of the two children of a split of area, plus the
// 1-cell-wide border between them.
func Split(area Rect, dir pane.SplitDirection, ratio float64) (Rect, Rect, Rect) {
	if dir == pane.SplitVertical {
		l, r := splitSizes(area.Cols, ratio)
		return Rect{area.Row, area.Col, area.Rows, l},
			Rect{area.Row, area.Col + l + 1, area.Rows, r},
			Rect{area.Row, area.Col + l, area.Rows, 1}
	}
	t, bt := splitSizes(area.Rows, ratio)
	return Rect{area.Row, area.Col, t, area.Cols},
		Rect{area.Row + t + 1, area.Col, bt, area.Cols},
		Rect{area.Row + t, area.Col, 1, area.Cols}
}

// Apply assigns positions and sizes to every pane in the tree and returns
// the border segments separating them.
func Apply(n *Node, area Rect) []Rect {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		n.Pane.SetRect(area.Row, area.Col, area.Rows, area.Cols)
		return nil
	}
	a, b, border := Split(area, n.Split, n.Ratio)
	borders := []Rect{border}
	borders = append(borders, Apply(n.Children[0], a)...)
	return append(borders, Apply(n.Children[1], b)...)
}

// DividerAt returns the split whose border runs through the cell (row,
// col), together with the area that split was laid out in. The area is
// what a ratio means: dragging the border to a column is only meaningful
// against the space the split had to divide.
func DividerAt(n *Node, area Rect, row, col int) (*Node, Rect, bool) {
	if n == nil || n.Pane != nil {
		return nil, Rect{}, false
	}
	a, b, border := Split(area, n.Split, n.Ratio)
	if border.Contains(row, col) {
		return n, area, true
	}
	if a.Contains(row, col) {
		return DividerAt(n.Children[0], a, row, col)
	}
	if b.Contains(row, col) {
		return DividerAt(n.Children[1], b, row, col)
	}
	return nil, Rect{}, false
}

// RatioAt is the ratio that puts a split's border at (row, col) of area.
// It is clamped so a drag can never collapse either side out of existence.
func RatioAt(n *Node, area Rect, row, col int) float64 {
	var r float64
	if n.Split == pane.SplitVertical {
		r = float64(col-area.Col) / float64(max(area.Cols, 1))
	} else {
		r = float64(row-area.Row) / float64(max(area.Rows, 1))
	}
	return min(max(r, 0.1), 0.9)
}

// FindLeaf returns the leaf node containing the given pane, or nil.
func FindLeaf(n *Node, p *pane.Pane) *Node {
	if n == nil {
		return nil
	}
	if n.Pane == p {
		return n
	}
	if l := FindLeaf(n.Children[0], p); l != nil {
		return l
	}
	return FindLeaf(n.Children[1], p)
}

// FindParent finds the parent node of the leaf containing p.
// Returns the parent and the child index (0 or 1), or nil, -1.
func FindParent(n *Node, p *pane.Pane) (*Node, int) {
	if n == nil {
		return nil, -1
	}
	for i, child := range n.Children {
		if child == nil {
			continue
		}
		if child.Pane == p {
			return n, i
		}
		if parent, idx := FindParent(child, p); parent != nil {
			return parent, idx
		}
	}
	return nil, -1
}

// RemovePane removes a pane from the tree, collapsing its parent so the
// sibling takes over the freed space. It returns the new root (nil when the
// tree is now empty) and the pane that should receive focus if the removed
// pane had it: the nearest pane in the sibling's subtree.
func RemovePane(root *Node, p *pane.Pane) (*Node, *pane.Pane) {
	if root == nil || root.Pane == p {
		return nil, nil
	}
	parent, idx := FindParent(root, p)
	if parent == nil {
		return root, nil
	}
	sibling := parent.Children[1-idx]
	*parent = *sibling
	// Moving into the space the removed pane vacated: the closest pane is
	// on the sibling subtree's edge that faced it.
	panes := CollectPanes(parent)
	if idx == 0 {
		return root, panes[0]
	}
	return root, panes[len(panes)-1]
}

// CollectPanes returns all panes in the tree in left-to-right, top-to-bottom order.
func CollectPanes(n *Node) []*pane.Pane {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return []*pane.Pane{n.Pane}
	}
	return append(CollectPanes(n.Children[0]), CollectPanes(n.Children[1])...)
}

// CountPanes returns the number of panes in the tree without allocating.
func CountPanes(n *Node) int {
	if n == nil {
		return 0
	}
	if n.Pane != nil {
		return 1
	}
	return CountPanes(n.Children[0]) + CountPanes(n.Children[1])
}

// AnyPane reports whether pred returns true for any pane in the tree.
func AnyPane(n *Node, pred func(*pane.Pane) bool) bool {
	if n == nil {
		return false
	}
	if n.Pane != nil {
		return pred(n.Pane)
	}
	return AnyPane(n.Children[0], pred) || AnyPane(n.Children[1], pred)
}

// PaneIndex returns the 0-based index of target in left-to-right, top-to-bottom
// order, or -1 if target is not in n.
func PaneIndex(n *Node, target *pane.Pane) int {
	idx, _ := paneIndex(n, target, 0)
	return idx
}

func paneIndex(n *Node, target *pane.Pane, cur int) (int, int) {
	if n == nil {
		return -1, cur
	}
	if n.Pane != nil {
		if n.Pane == target {
			return cur, cur + 1
		}
		return -1, cur + 1
	}
	if f, n0 := paneIndex(n.Children[0], target, cur); f >= 0 {
		return f, n0
	} else {
		return paneIndex(n.Children[1], target, n0)
	}
}

// Equalize balances split ratios so sibling panes along the same split
// direction receive equal space (e.g. 3 sequential vertical splits become
// 1/3 each instead of 1/2, 1/4, 1/4).
func Equalize(n *Node) {
	if n == nil || n.Pane != nil {
		return
	}
	left := spanCount(n.Children[0], n.Split)
	right := spanCount(n.Children[1], n.Split)
	if total := left + right; total > 0 {
		n.Ratio = float64(left) / float64(total)
	} else {
		n.Ratio = 0.5
	}
	Equalize(n.Children[0])
	Equalize(n.Children[1])
}

func spanCount(n *Node, dir pane.SplitDirection) int {
	if n == nil {
		return 0
	}
	if n.Pane != nil || n.Split != dir {
		return 1
	}
	return spanCount(n.Children[0], dir) + spanCount(n.Children[1], dir)
}

// Arrange builds a binary layout tree for panes according to mode.
// For ModeStack or a single pane, it returns an even-horizontal tree.
func Arrange(panes []*pane.Pane, mode Mode) *Node {
	if len(panes) == 0 {
		return nil
	}
	if len(panes) == 1 {
		return NewLeaf(panes[0])
	}
	switch mode {
	case ModeEvenVertical:
		return splitChain(panes, pane.SplitHorizontal)
	case ModeMasterLeft:
		return &Node{
			Split:    pane.SplitVertical,
			Ratio:    0.5,
			Children: [2]*Node{NewLeaf(panes[0]), splitChain(panes[1:], pane.SplitHorizontal)},
		}
	case ModeMasterRight:
		return &Node{
			Split:    pane.SplitVertical,
			Ratio:    0.5,
			Children: [2]*Node{splitChain(panes[1:], pane.SplitHorizontal), NewLeaf(panes[0])},
		}
	case ModeMasterTop:
		return &Node{
			Split:    pane.SplitHorizontal,
			Ratio:    0.5,
			Children: [2]*Node{NewLeaf(panes[0]), splitChain(panes[1:], pane.SplitVertical)},
		}
	case ModeMasterBottom:
		return &Node{
			Split:    pane.SplitHorizontal,
			Ratio:    0.5,
			Children: [2]*Node{splitChain(panes[1:], pane.SplitVertical), NewLeaf(panes[0])},
		}
	case ModeGrid:
		return arrangeGrid(panes)
	case ModeEvenHorizontal, ModeStack:
		return splitChain(panes, pane.SplitVertical)
	default:
		return splitChain(panes, pane.SplitVertical)
	}
}

func splitChain(panes []*pane.Pane, dir pane.SplitDirection) *Node {
	nodes := make([]*Node, len(panes))
	for i, p := range panes {
		nodes[i] = NewLeaf(p)
	}
	return joinNodes(nodes, dir)
}

func joinNodes(nodes []*Node, dir pane.SplitDirection) *Node {
	n := len(nodes)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return nodes[0]
	}
	return &Node{
		Split:    dir,
		Ratio:    1.0 / float64(n),
		Children: [2]*Node{nodes[0], joinNodes(nodes[1:], dir)},
	}
}

func arrangeGrid(panes []*pane.Pane) *Node {
	n := len(panes)
	cols := int(math.Ceil(math.Sqrt(float64(n))))
	var rowNodes []*Node
	for i := 0; i < n; i += cols {
		end := min(i+cols, n)
		rowNodes = append(rowNodes, splitChain(panes[i:end], pane.SplitVertical))
	}
	return joinNodes(rowNodes, pane.SplitHorizontal)
}
