package layout

import (
	"github.com/noturbob/slat/internal/pane"
)

// Rect represents a rectangular region of the terminal.
type Rect struct {
	Row  int
	Col  int
	Rows int
	Cols int
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
	return &Node{
		Split: pane.SplitNone,
		Pane:  p,
		Ratio: 0.5,
	}
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

// Apply recursively assigns positions and sizes to all panes in the tree.
func Apply(n *Node, area Rect) {
	if n == nil {
		return
	}

	if n.Pane != nil {
		// Leaf node - assign the area to the pane
		rows := uint16(area.Rows)
		cols := uint16(area.Cols)
		if rows < 1 {
			rows = 1
		}
		if cols < 1 {
			cols = 1
		}
		n.Pane.SetPosition(area.Row, area.Col)
		_ = n.Pane.Resize(rows, cols)
		return
	}

	// Internal node - split the area between children
	switch n.Split {
	case pane.SplitVertical:
		leftCols := int(float64(area.Cols) * n.Ratio)
		if leftCols < 1 {
			leftCols = 1
		}
		rightCols := area.Cols - leftCols - 1 // -1 for the border column
		if rightCols < 1 {
			rightCols = 1
		}
		Apply(n.Children[0], Rect{area.Row, area.Col, area.Rows, leftCols})
		Apply(n.Children[1], Rect{area.Row, area.Col + leftCols + 1, area.Rows, rightCols})

	case pane.SplitHorizontal:
		topRows := int(float64(area.Rows) * n.Ratio)
		if topRows < 1 {
			topRows = 1
		}
		bottomRows := area.Rows - topRows - 1 // -1 for the border row
		if bottomRows < 1 {
			bottomRows = 1
		}
		Apply(n.Children[0], Rect{area.Row, area.Col, topRows, area.Cols})
		Apply(n.Children[1], Rect{area.Row + topRows + 1, area.Col, bottomRows, area.Cols})
	}
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
	for i := 0; i < 2; i++ {
		child := n.Children[i]
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

// RemovePane removes a pane from the tree and collapses the parent.
// Returns the new root (may be nil if tree is now empty).
func RemovePane(root *Node, p *pane.Pane) *Node {
	if root == nil {
		return nil
	}
	// If the root IS the pane leaf
	if root.Pane == p {
		return nil
	}

	parent, idx := FindParent(root, p)
	if parent == nil {
		return root
	}

	// The sibling takes the parent's place
	siblingIdx := 1 - idx
	sibling := parent.Children[siblingIdx]

	parent.Split = sibling.Split
	parent.Ratio = sibling.Ratio
	parent.Pane = sibling.Pane
	parent.Children = sibling.Children

	return root
}

// CollectPanes returns all panes in the tree in left-to-right, top-to-bottom order.
func CollectPanes(n *Node) []*pane.Pane {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return []*pane.Pane{n.Pane}
	}
	var result []*pane.Pane
	result = append(result, CollectPanes(n.Children[0])...)
	result = append(result, CollectPanes(n.Children[1])...)
	return result
}
