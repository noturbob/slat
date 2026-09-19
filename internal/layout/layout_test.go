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
