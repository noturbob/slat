//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/ui"
)

// A pane's last line is output from whatever is running in it, so a hook
// must treat it as text. If it reached /bin/sh unquoted, this would run.
func TestHookQuotesPaneText(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	log := filepath.Join(dir, "log")

	a := &App{cfg: config.DefaultConfig()}
	a.hook("echo %p %c >> "+log, PaneInfo{
		Pane: 7,
		Last: `save it'; touch ` + marker + `; echo 'ok? [y/N]`,
	})()

	var got string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(log); err == nil && len(b) > 0 {
			got = string(b)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the pane's last line was executed as a command")
	}
	if !strings.Contains(got, "7 save it'; touch") {
		t.Errorf("hook logged %q", got)
	}
}

func TestHookUnsetDoesNothing(t *testing.T) {
	a := &App{cfg: config.DefaultConfig()}
	a.hook("", PaneInfo{Pane: 1})() // must not panic or run a shell
	a.hook("   ", PaneInfo{Pane: 1})()
}

// A pane waiting for input marks its tab in the status bar, so a human
// watching an agent can see it from another tab.
func TestStatusBarMarksTabWaitingForInput(t *testing.T) {
	a, term := start(t, func(c *config.Config) {
		c.Agent.InputAfter = config.Duration(300 * time.Millisecond)
	})
	a.FeedInput([]byte{prefix, 'c'}) // new tab
	waitFor(t, term, "2:shell", 1)
	a.FeedInput([]byte("sh -c 'printf \"Continue? [y/N] \"; read answer'\r"))
	waitFor(t, term, "Continue? [y/N]", 1)

	a.FeedInput([]byte{prefix, 'p'}) // back to the first tab
	waitFor(t, term, "2:shell ?", 1)

	// Answering it clears the mark.
	a.FeedInput([]byte{prefix, 'n'})
	a.FeedInput([]byte("y\r"))
	waitGone(t, term, "2:shell ?")
}

// The whole theming path: config table -> ui.Theme -> painted cells.
func TestThemeReachesTheScreen(t *testing.T) {
	settings := map[string]string{"name": "nord", "accent": "#ff0000"}
	_, term := start(t, func(c *config.Config) { c.Theme = settings })
	want, err := ui.ParseTheme(settings)
	if err != nil {
		t.Fatal(err)
	}

	// The status bar is the bottom row, and the workspace name on it is
	// drawn in the accent colour.
	bar := term.cell(0, 29)
	if bar.Style.Bg != want.Bg {
		t.Errorf("status bar background = %v, want %v", bar.Style.Bg, want.Bg)
	}
	if name := term.cell(1, 29); name.Style.Fg != want.Accent {
		t.Errorf("workspace name colour = %v, want the accent %v", name.Style.Fg, want.Accent)
	}
}

// A new pane is revealed over a moment instead of snapping in. The pane is
// at its final size the whole time, so nothing is resized twice.
func TestSplitAnimation(t *testing.T) {
	a, term := start(t, func(c *config.Config) {
		c.Animation.Split = config.Duration(400 * time.Millisecond)
	})
	before := a.Panes()[0]

	a.FeedInput([]byte{prefix, 'v'}) // split left/right
	waitFor(t, term, "░", 1)         // the curtain over the new pane
	waitGone(t, term, "░")           // and it's gone once revealed
	waitFor(t, term, "slat$", 2)     // both prompts on screen

	// The panes kept their size while the curtain moved: a program in a
	// pane must not be resized frame by frame.
	after := a.Panes()
	if len(after) != 2 {
		t.Fatalf("expected two panes, got %d", len(after))
	}
	if after[0].Cols >= before.Cols {
		t.Errorf("the split pane didn't shrink: %d -> %d", before.Cols, after[0].Cols)
	}
	if after[1].Rows != after[0].Rows {
		t.Errorf("a left/right split gave different heights: %+v", after)
	}
}

// Animations can be switched off, and then nothing is drawn over a pane.
func TestAnimationOff(t *testing.T) {
	a, term := start(t, func(c *config.Config) {
		c.Animation.Split, c.Animation.Move = 0, 0
	})
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)
	if strings.Contains(term.String(), "░") {
		t.Errorf("animation drawn although it is off:\n%s", term.String())
	}
}

// A shell's own prompt must not be mistaken for a question, or every idle
// pane would look like it needs attention.
func TestShellPrompt(t *testing.T) {
	prompts := []string{
		"$ ", "slat$", "[user@host dir]$ ", "root@box:/# ", "zsh%",
		"~/src/slat ❯ ", "user in slat ?1 ❯", "PS1>", "λ ",
	}
	for _, p := range prompts {
		if !shellPrompt(p) {
			t.Errorf("shellPrompt(%q) = false, want true", p)
		}
	}
	questions := []string{
		"Overwrite? [y/N] ", "Continue (y/n)?", "Password:",
		"Press enter to continue", "Delete 3 files?", "",
	}
	for _, q := range questions {
		if shellPrompt(q) {
			t.Errorf("shellPrompt(%q) = true, want false", q)
		}
	}
}

// column reports where s sits on screen, or -1. Panes are laid out side by
// side, so the column says which pane holds the text.
func column(term *terminal, s string) int {
	for _, line := range strings.Split(term.String(), "\n") {
		if i := strings.Index(line, s); i >= 0 {
			return i
		}
	}
	return -1
}

func waitColumn(t *testing.T, term *terminal, s string, leftOfHalf bool) int {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if c := column(term, s); c >= 0 && (c < 50) == leftOfHalf {
			return c
		}
		time.Sleep(50 * time.Millisecond)
	}
	side := "the right half"
	if leftOfHalf {
		side = "the left half"
	}
	t.Fatalf("%q never reached %s (column %d); screen:\n%s", s, side, column(term, s), term.String())
	return -1
}

// A pane can be walked around the layout, the way a tiling window manager
// moves a window: the two panes trade places and focus follows the one that
// moved.
func TestMovePaneInDirection(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'}) // split left/right; focus moves right
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte("echo RIGHTMARK\r"))
	waitColumn(t, term, "RIGHTMARK", false)

	a.FeedInput([]byte{prefix, 'H'}) // focus the left pane
	a.FeedInput([]byte("echo LEFTMARK\r"))
	waitColumn(t, term, "LEFTMARK", true)

	// Move the focused (left) pane to the right: they swap sides.
	a.FeedInput([]byte{prefix, '>'})
	waitColumn(t, term, "LEFTMARK", false)
	waitColumn(t, term, "RIGHTMARK", true)

	// Focus followed the pane that moved, so typing lands in it.
	a.FeedInput([]byte("echo STILLFOCUSED\r"))
	waitColumn(t, term, "STILLFOCUSED", false)

	// And back again.
	a.FeedInput([]byte{prefix, '<'})
	waitColumn(t, term, "LEFTMARK", true)
}

// Move mode walks the pane with unprefixed keys and says so in the bar.
func TestMoveMode(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte("echo WALKER\r"))
	waitColumn(t, term, "WALKER", false)

	a.FeedInput([]byte{prefix, 'm'})
	waitFor(t, term, "MOVE", 1) // the badge

	// No prefix needed: h moves it left, l brings it back.
	a.FeedInput([]byte("h"))
	waitColumn(t, term, "WALKER", true)
	a.FeedInput([]byte("l"))
	waitColumn(t, term, "WALKER", false)

	// q leaves, and the key isn't passed to the shell.
	a.FeedInput([]byte("q"))
	waitGone(t, term, "MOVE")
	a.FeedInput([]byte("echo AFTERWARDS\r"))
	waitColumn(t, term, "AFTERWARDS", false)
	if strings.Contains(term.String(), "qecho") {
		t.Error("the q that left move mode reached the shell")
	}
}
