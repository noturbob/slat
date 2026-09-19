package layout

import (
	"github.com/noturbob/slat/internal/pane"
)

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
func splitSizes(total int, ratio float64) (first, second int) {
	first = int(float64(total) * ratio)
	first = min(max(first, 1), total-2)
	first = max(first, 1)
	second = max(total-first-1, 0)
	return first, second
}

// Split returns the rects of the two children of a split of area, plus the
// 1-cell-wide border between them.
func Split(area Rect, dir pane.SplitDirection, ratio float64) (a, b, border Rect) {
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
func Apply(n *Node, area Rect) (borders []Rect) {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		n.Pane.SetRect(area.Row, area.Col, area.Rows, area.Cols)
		return nil
	}
	a, b, border := Split(area, n.Split, n.Ratio)
	borders = append(borders, border)
	borders = append(borders, Apply(n.Children[0], a)...)
	return append(borders, Apply(n.Children[1], b)...)
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
func RemovePane(root *Node, p *pane.Pane) (newRoot *Node, heir *pane.Pane) {
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

// Equalize recursively resets all split ratios to 0.5.
func Equalize(n *Node) {
	if n == nil || n.Pane != nil {
		return
	}
	n.Ratio = 0.5
	Equalize(n.Children[0])
	Equalize(n.Children[1])
}
