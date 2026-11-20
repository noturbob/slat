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

func TestEqualize(t *testing.T) {
	// 3 sequential vertical splits: [p1 | [p2 | p3]] should become 1/3 each.
	p1, p2, p3 := &pane.Pane{ID: 1}, &pane.Pane{ID: 2}, &pane.Pane{ID: 3}
	root := &Node{
		Split: pane.SplitVertical,
		Ratio: 0.5,
		Children: [2]*Node{
			NewLeaf(p1),
			{
				Split:    pane.SplitVertical,
				Ratio:    0.5,
				Children: [2]*Node{NewLeaf(p2), NewLeaf(p3)},
			},
		},
	}
	Equalize(root)
	a1, b1, _ := Split(Rect{Rows: 24, Cols: 92}, root.Split, root.Ratio)
	a2, b2, _ := Split(b1, root.Children[1].Split, root.Children[1].Ratio)
	if a1.Cols != 30 || a2.Cols != 30 || b2.Cols != 30 {
		t.Errorf("3-pane equalize cols = %d, %d, %d; want 30, 30, 30", a1.Cols, a2.Cols, b2.Cols)
	}

	// Mixed split: [p1 | [p2 / p3]] -> p1 gets half width; p2 and p3 share the other half vertically.
	mix := &Node{
		Split: pane.SplitVertical,
		Ratio: 0.2,
		Children: [2]*Node{
			NewLeaf(p1),
			{
				Split:    pane.SplitHorizontal,
				Ratio:    0.2,
				Children: [2]*Node{NewLeaf(p2), NewLeaf(p3)},
			},
		},
	}
	Equalize(mix)
	ma, mb, _ := Split(Rect{Rows: 21, Cols: 81}, mix.Split, mix.Ratio)
	m2, m3, _ := Split(mb, mix.Children[1].Split, mix.Children[1].Ratio)
	if ma.Cols != 40 || mb.Cols != 40 || m2.Rows != 10 || m3.Rows != 10 {
		t.Errorf("mixed equalize got p1_c=%d p2=(%d,%d) p3=(%d,%d)", ma.Cols, m2.Rows, m2.Cols, m3.Rows, m3.Cols)
	}
}

func TestParseMode(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
		ok   bool
	}{
		{"master-left", ModeMasterLeft, true},
		{"master_right", ModeMasterRight, true},
		{"MASTER TOP", ModeMasterTop, true},
		{"master-bottom", ModeMasterBottom, true},
		{"even-horizontal", ModeEvenHorizontal, true},
		{"even_vertical", ModeEvenVertical, true},
		{"grid", ModeGrid, true},
		{"stack", ModeStack, true},
		{"unknown", "", false},
	}
	for _, tc := range cases {
		got, ok := ParseMode(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseMode(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if len(AllModes()) != 8 {
		t.Errorf("AllModes() len = %d, want 8", len(AllModes()))
	}
}

func collectRects(n *Node, area Rect) (rects, borders []Rect) {
	if n == nil {
		return nil, nil
	}
	if n.Pane != nil {
		return []Rect{area}, nil
	}
	a, b, border := Split(area, n.Split, n.Ratio)
	borders = append(borders, border)
	ra, ba := collectRects(n.Children[0], a)
	rb, bb := collectRects(n.Children[1], b)
	rects = append(ra, rb...)
	borders = append(borders, ba...)
	borders = append(borders, bb...)
	return rects, borders
}

func TestArrangeModes(t *testing.T) {
	area := Rect{Row: 0, Col: 0, Rows: 40, Cols: 120}
	for _, info := range AllModes() {
		for count := 1; count <= 5; count++ {
			panes := make([]*pane.Pane, count)
			for i := range panes {
				panes[i] = &pane.Pane{ID: i + 1}
			}
			root := Arrange(panes, info.Mode)
			got := CollectPanes(root)
			if len(got) != count {
				t.Fatalf("mode %s count %d: CollectPanes = %d", info.Mode, count, len(got))
			}
			rects, borders := collectRects(root, area)
			if len(rects) != count {
				t.Fatalf("mode %s count %d: rects = %d", info.Mode, count, len(rects))
			}
			cells := 0
			for i, r := range rects {
				if r.Rows < 1 || r.Cols < 1 {
					t.Fatalf("mode %s count %d: pane %d empty rect %+v", info.Mode, count, i, r)
				}
				cells += r.Rows * r.Cols
			}
			for _, b := range borders {
				cells += b.Rows * b.Cols
			}
			if cells != area.Rows*area.Cols {
				t.Fatalf("mode %s count %d: covered %d != %d", info.Mode, count, cells, area.Rows*area.Cols)
			}
		}
	}
}
