package ui

var (
	Themes           = themes
	StyleBar         = &styleBar
	StyleTab         = &styleTab
	StyleBox         = &styleBox
	StyleBoxHead     = &styleBoxHead
	StyleBoxKey      = &styleBoxKey
	StyleBoxText     = &styleBoxText
	StyleFinder         = &styleFinder
	StyleFinderKey      = &styleFinderKey
	StyleFinderSel      = &styleFinderSel
	StyleFinderMatch    = &styleFinderMatch
	StyleFinderSelMatch = &styleFinderSelMatch
	StyleStack       = &styleStack
	StyleStackActive = &styleStackActive
)

// ClearCompiledForBench resets compiled format state for uncompiled benchmarks.
func ClearCompiledForBench(f *StatusFormat) {
	f.compiled = nil
}
