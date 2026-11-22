package vt

// PushHistoryForBench exposes pushHistory to external vt_test benchmarks.
func PushHistoryForBench(t *Terminal, line []Cell) {
	t.pushHistory(line)
}
