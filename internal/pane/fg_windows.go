package pane

// Windows' ConPTY has no foreground process group, so pane status there
// is derived from output activity alone (see Pane.Activity).
func foreground(fd uintptr) (int, string) { return 0, "" }
