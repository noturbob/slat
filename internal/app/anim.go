package app

import (
	"slices"
	"time"

	"github.com/noturbob/slat/internal/layout"
	"github.com/noturbob/slat/internal/pane"
	"github.com/noturbob/slat/internal/ui"
)

// anim is a pane appearing. Only the drawing is animated: the pane is
// already its final size, so no program is resized more than once.
type anim struct {
	pane  *pane.Pane
	dir   pane.SplitDirection
	start time.Time
}

// startAnim begins revealing a newly created pane. Called with the lock
// held; a no-op when animations are switched off.
func (a *App) startAnim(p *pane.Pane, dir pane.SplitDirection) {
	if p == nil || a.cfg.Animation.Split.D() <= 0 || a.cfg.Animation.Reveal == "none" {
		return
	}
	a.anim = &anim{pane: p, dir: dir, start: time.Now()}
}

// drawAnim covers the part of the new pane that hasn't been revealed yet
// and asks for another frame. It reports whether the animation is still
// running.
func (a *App) drawAnim(frame *ui.Frame, visible []*pane.Pane, now time.Time) bool {
	if !slices.Contains(visible, a.anim.pane) {
		return false // closed, zoomed over, or on another tab now
	}
	progress := float64(now.Sub(a.anim.start)) / float64(a.cfg.Animation.Split.D())
	if progress >= 1 {
		return false
	}
	progress = a.cfg.Animation.Ease(progress)
	row, col, rows, cols := a.anim.pane.Rect()
	r := layout.Rect{Row: row, Col: col, Rows: rows, Cols: cols}
	sideways := a.anim.dir == pane.SplitVertical // left/right
	span := rows
	if sideways {
		span = cols
	}
	ui.Curtain(frame, r, sideways, int(progress*float64(span)), a.cfg.Animation.RevealGlyph())

	// Nothing else is producing frames, so the animation drives itself.
	time.AfterFunc(frameInterval, a.markDirty)
	return true
}
