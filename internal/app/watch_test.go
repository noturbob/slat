//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
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
