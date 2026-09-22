//go:build !windows

package app

import (
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/config"
)

// clipboard returns what slat last asked the terminal to copy, by decoding
// the OSC 52 sequence out of the raw output.
func clipboard(t *testing.T, term *terminal) string {
	t.Helper()
	re := regexp.MustCompile(`\x1b]52;c;([A-Za-z0-9+/=]*)\a`)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if m := re.FindAllStringSubmatch(term.rawString(), -1); len(m) > 0 {
			text, err := base64.StdEncoding.DecodeString(m[len(m)-1][1])
			if err != nil {
				t.Fatalf("OSC 52 payload is not base64: %v", err)
			}
			return string(text)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ""
}

// The whole path: scroll back, find a line, select it, copy it — and the
// text lands on the terminal's clipboard rather than on the screen.
func TestCopyModeYanksALine(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("echo COPY-ME-PLEASE\r"))
	waitFor(t, term, "COPY-ME-PLEASE", 2)

	a.FeedInput([]byte{prefix, '['}) // scroll mode
	waitFor(t, term, "SCROLL", 1)

	a.FeedInput([]byte("/"))                // search, which moves the cursor
	a.FeedInput([]byte("COPY-ME-PLEASE\r")) //
	a.FeedInput([]byte("V"))                // select the whole line
	waitFor(t, term, "COPY", 1)             // the badge says so
	a.FeedInput([]byte("y"))                // yank

	got := clipboard(t, term)
	if !strings.Contains(got, "COPY-ME-PLEASE") {
		t.Errorf("clipboard = %q, want it to contain the line", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("a line selection should end with a newline: %q", got)
	}
	// Yanking leaves copy mode, and the text was never painted anywhere.
	waitGone(t, term, "SCROLL")
	if strings.Count(term.String(), "COPY-ME-PLEASE") > 2 {
		t.Error("the yanked text was drawn onto the screen")
	}
}

// A character selection copies exactly what is highlighted, without the
// padding a terminal line carries to the right edge.
func TestCopyModeCharacterSelection(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte("echo abcdefghij\r"))
	waitFor(t, term, "abcdefghij", 2)

	a.FeedInput([]byte{prefix, '['})
	a.FeedInput([]byte("/"))
	a.FeedInput([]byte("abcdefghij\r")) // cursor lands on the 'a'
	a.FeedInput([]byte("v"))            // start a character selection
	for i := 0; i < 4; i++ {
		a.FeedInput([]byte("l")) // extend over abcde
	}
	a.FeedInput([]byte("y"))

	if got := clipboard(t, term); got != "abcde" {
		t.Errorf("clipboard = %q, want %q", got, "abcde")
	}
}

// Escape drops the selection but stays in scroll mode; a second one leaves.
func TestCopyModeEscapeSteps(t *testing.T) {
	a, term := start(t)
	a.FeedInput([]byte{prefix, '['})
	waitFor(t, term, "SCROLL", 1)
	a.FeedInput([]byte("V"))
	waitFor(t, term, "COPY", 1)
	a.FeedInput([]byte{0x1b})
	waitFor(t, term, "SCROLL", 1) // selection gone, still scrolling
	a.FeedInput([]byte{0x1b})
	waitGone(t, term, "SCROLL")
}

// The optional local command gets the same text on its standard input,
// for terminals that refuse OSC 52.
func TestCopyCommandReceivesTheText(t *testing.T) {
	dir := t.TempDir()
	out := dir + "/clip.txt"
	a, term := start(t, func(c *config.Config) {
		c.Copy.Command = "cat > " + out
	})
	a.FeedInput([]byte("echo VIA-COMMAND\r"))
	waitFor(t, term, "VIA-COMMAND", 2)
	a.FeedInput([]byte{prefix, '['})
	a.FeedInput([]byte("/"))
	a.FeedInput([]byte("VIA-COMMAND\r"))
	a.FeedInput([]byte("V"))
	a.FeedInput([]byte("y"))

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if b, err := os.ReadFile(out); err == nil && strings.Contains(string(b), "VIA-COMMAND") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("the copy command never received the text")
}
