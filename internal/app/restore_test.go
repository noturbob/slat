//go:build !windows

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/noturbob/slat/internal/app"

	"github.com/noturbob/slat/internal/config"
	"github.com/noturbob/slat/internal/session"
)

// The promise of the feature: output printed before the daemon died is on
// screen when it comes back, in the same layout.
func TestSessionSurvivesTheDaemon(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'}) // two panes
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte("echo before-the-reboot\r"))
	waitFor(t, term, "before-the-reboot", 1)

	// Anything that is not the user quitting: the session is saved.
	a.Shutdown()

	path, err := session.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("no session was saved: %v", statErr)
	}

	b, term2 := start(t)
	defer b.Shutdown()
	if n := len(activePanes(b)); n != 2 {
		t.Fatalf("restored %d panes, want 2", n)
	}
	waitFor(t, term2, "before-the-reboot", 1)
	waitFor(t, term2, "restored", 1) // the rule under the old output
}

// Quitting is a decision. A session that came back afterwards would be a
// bug, not a feature.
func TestQuitForgetsTheSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	a, term := start(t)
	a.FeedInput([]byte("echo doomed\r"))
	waitFor(t, term, "doomed", 1)
	a.SaveSessionForTest() // as the ticker would have

	path, _ := session.StatePath()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("nothing saved to forget: %v", err)
	}

	a.FeedInput([]byte{prefix, 'q'})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("quit left the session behind (err = %v)", err)
	}

	b, term2 := start(t)
	defer b.Shutdown()
	if n := len(activePanes(b)); n != 1 {
		t.Fatalf("after quitting, a new session has %d panes, want 1", n)
	}
	if s := term2.String(); strings.Contains(s, "doomed") {
		t.Fatal("the quit session came back")
	}
}

// restore = false means a clean session every time.
func TestRestoreCanBeTurnedOff(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	a, term := start(t)
	a.FeedInput([]byte("echo remembered\r"))
	waitFor(t, term, "remembered", 1)
	a.Shutdown()

	b, term2 := start(t, func(c *config.Config) { c.Restore = false })
	defer b.Shutdown()
	if s := term2.String(); strings.Contains(s, "remembered") {
		t.Fatal("restore = false still brought the old session back")
	}
}

// A file from a future slat, or a corrupt one, must not stop slat starting.
func TestUnreadableStateStartsCleanly(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	path, err := session.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	for _, content := range []string{`{"version":99,"workspaces":[{}]}`, `not json at all`, `{}`} {
		if writeErr := os.WriteFile(path, []byte(content), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		a, _ := start(t)
		if n := len(activePanes(a)); n != 1 {
			t.Fatalf("state %q: got %d panes, want a clean session of 1", content, n)
		}
		a.Shutdown()
	}
}

// The layout comes back, not just the panes: a saved tree of three panes
// returns as three panes in the same shape.
func TestRestoreKeepsTheLayout(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	a, term := start(t)
	a.FeedInput([]byte{prefix, 'v'})
	waitFor(t, term, "slat$", 2)
	a.FeedInput([]byte{prefix, 'h'})
	waitFor(t, term, "slat$", 3)
	before := paneRects(a)
	a.Shutdown()

	b, _ := start(t)
	defer b.Shutdown()
	after := paneRects(b)
	if len(after) != len(before) {
		t.Fatalf("restored %d panes, want %d", len(after), len(before))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("pane %d came back at %v, was at %v", i, after[i], before[i])
		}
	}
}

type rect struct{ row, col, rows, cols int }

func paneRects(a *App) []rect {
	var out []rect
	for _, p := range activePanes(a) {
		r, c, rows, cols := p.Rect()
		out = append(out, rect{r, c, rows, cols})
	}
	return out
}

func TestAgo(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "moments ago"},
		{1 * time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{90 * time.Minute, "1 hour ago"},
		{72 * time.Hour, "3 days ago"},
	}
	for _, c := range cases {
		if got := Ago(time.Now().Add(-c.d)); got != c.want {
			t.Errorf("ago(%s) = %q, want %q", c.d, got, c.want)
		}
	}
	if got := Ago(time.Time{}); got != "an earlier session" {
		t.Errorf("ago(zero) = %q", got)
	}
}
