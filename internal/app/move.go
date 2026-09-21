package app

import (
	"time"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/ui"
)

// slide is two panes trading places. Only the painting moves: the layout
// has already given both panes their final size and position, so each is
// resized at most once, as it would be without the animation — the frames
// in between just paint them on their way across. Neighbours in a split
// differ by the border column, so this can't require equal sizes.
type slide struct {
	a, b         *pane.Pane
	aFrom, bFrom layout.Rect
	aTo, bTo     layout.Rect
	start        time.Time
}

// moveKeys are the keys that work inside move mode, on top of the usual
// direction keys, so the pane can be walked without the prefix each time.
var moveKeys = map[byte]string{
	'h': "left", 'l': "right", 'k': "up", 'j': "down",
	'H': "left", 'L': "right", 'K': "up", 'J': "down",
}

// movePane swaps the active pane with its neighbour in that direction and
// animates the exchange. Called with the lock held.
func (a *App) movePane(direction string) {
	before := map[*pane.Pane]layout.Rect{}
	for _, p := range a.manager.ActivePanes() {
		before[p] = rectOf(p)
	}
	moved, other, ok := a.manager.MovePaneInDirection(direction)
	if !ok {
		return
	}
	// The layout hasn't been applied yet, so the new rectangles are the
	// ones the two panes are about to swap into: each other's.
	a.slide = &slide{
		a: moved, b: other,
		aFrom: before[moved], bFrom: before[other],
		aTo: before[other], bTo: before[moved],
		start: time.Now(),
	}
	a.markDirty()
}

// drawSlide paints the two moving panes on their way across, and reports
// whether the animation is still running. Called with the lock held.
func (a *App) drawSlide(frame *ui.Frame, now time.Time) bool {
	s := a.slide
	if a.cfg.Animate.D() <= 0 {
		return false
	}
	progress := float64(now.Sub(s.start)) / float64(a.cfg.Animate.D())
	if progress >= 1 {
		return false
	}
	if progress < 0 {
		progress = 0
	}
	s.a.DrawAt(frame.Lines, lerp(s.aFrom.Row, s.aTo.Row, progress), lerp(s.aFrom.Col, s.aTo.Col, progress))
	s.b.DrawAt(frame.Lines, lerp(s.bFrom.Row, s.bTo.Row, progress), lerp(s.bFrom.Col, s.bTo.Col, progress))

	// Nothing else is producing frames, so the animation drives itself.
	time.AfterFunc(frameInterval, a.markDirty)
	return true
}

// sliding reports whether p is one of the panes currently in flight, which
// is how render knows to leave it to drawSlide.
func (s *slide) sliding(p *pane.Pane) bool { return s != nil && (p == s.a || p == s.b) }

func rectOf(p *pane.Pane) layout.Rect {
	row, col, rows, cols := p.Rect()
	return layout.Rect{Row: row, Col: col, Rows: rows, Cols: cols}
}

func lerp(from, to int, progress float64) int {
	return from + int(float64(to-from)*progress+0.5)
}
