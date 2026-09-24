package layout

import (
	"testing"

	"github.com/noturbob/slat/internal/pane"
)

// Children plus the border always tile the area exactly, with no overlap,
// at every ratio and down to tiny sizes.
func TestSplitTilesArea(t *testing.T) {
	for _, dir := range []pane.SplitDirection{pane.SplitVertical, pane.SplitHorizontal} {
		for size := 3; size <= 50; size++ {
			for _, ratio := range []float64{0.1, 0.33, 0.5, 0.9} {
				area := Rect{Row: 2, Col: 3, Rows: size, Cols: size}
				a, b, border := Split(area, dir, ratio)
				for _, r := range []Rect{a, b, border} {
					if r.Rows < 1 || r.Cols < 1 {
						t.Fatalf("dir %d size %d ratio %v: empty rect %+v", dir, size, ratio, r)
					}
				}
				covered := a.Rows*a.Cols + b.Rows*b.Cols + border.Rows*border.Cols
				if covered != size*size {
					t.Fatalf("dir %d size %d ratio %v: covers %d of %d cells", dir, size, ratio, covered, size*size)
				}
				if !area.Contains(b.Row+b.Rows-1, b.Col+b.Cols-1) {
					t.Fatalf("second child %+v spills out of %+v", b, area)
				}
			}
		}
	}
}

func TestDividerAt(t *testing.T) {
	area := Rect{Row: 0, Col: 0, Rows: 20, Cols: 80}
	// One vertical split: the border sits at the column Split() reports.
	root := &Node{Split: pane.SplitVertical, Ratio: 0.5,
		Children: [2]*Node{{}, {}}}
	_, _, border := Split(area, root.Split, root.Ratio)

	n, got, ok := DividerAt(root, area, border.Row, border.Col)
	if !ok || n != root || got != area {
		t.Fatalf("DividerAt on the border = %v, %v, %v", n, got, ok)
	}
	// A cell inside a child is not a divider.
	if _, _, ok := DividerAt(root, area, 5, 2); ok {
		t.Error("a cell inside the left pane was reported as a divider")
	}
	if _, _, ok := DividerAt(root, area, 5, border.Col+3); ok {
		t.Error("a cell inside the right pane was reported as a divider")
	}
}

// A nested split's own divider must be found, not its parent's.
func TestDividerAtNested(t *testing.T) {
	area := Rect{Row: 0, Col: 0, Rows: 20, Cols: 80}
	inner := &Node{Split: pane.SplitHorizontal, Ratio: 0.5, Children: [2]*Node{{}, {}}}
	root := &Node{Split: pane.SplitVertical, Ratio: 0.5, Children: [2]*Node{{}, inner}}

	_, right, _ := Split(area, root.Split, root.Ratio)
	_, _, innerBorder := Split(right, inner.Split, inner.Ratio)

	n, got, ok := DividerAt(root, area, innerBorder.Row, innerBorder.Col)
	if !ok || n != inner {
		t.Fatalf("nested divider: got node %v ok %v, want the inner split", n, ok)
	}
	if got != right {
		t.Errorf("nested divider area = %v, want the right half %v", got, right)
	}
}

func TestRatioAt(t *testing.T) {
	area := Rect{Row: 0, Col: 0, Rows: 20, Cols: 80}
	v := &Node{Split: pane.SplitVertical}
	if got := RatioAt(v, area, 0, 40); got != 0.5 {
		t.Errorf("vertical ratio at the middle column = %v, want 0.5", got)
	}
	h := &Node{Split: pane.SplitHorizontal}
	if got := RatioAt(h, area, 5, 0); got != 0.25 {
		t.Errorf("horizontal ratio at row 5 of 20 = %v, want 0.25", got)
	}
	// Dragging to the edge must leave both sides alive.
	if got := RatioAt(v, area, 0, 0); got != 0.1 {
		t.Errorf("ratio at the far left = %v, want it clamped to 0.1", got)
	}
	if got := RatioAt(v, area, 0, 79); got != 0.9 {
		t.Errorf("ratio at the far right = %v, want it clamped to 0.9", got)
	}
}
