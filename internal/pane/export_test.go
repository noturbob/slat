//go:build !windows

package pane

import "github.com/noturbob/slat/internal/vt"

// NewForBench constructs a Pane backed only by a vt.Terminal for benchmarks.
func NewForBench(term *vt.Terminal) *Pane {
	return &Pane{term: term}
}
