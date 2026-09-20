package app

import "testing"

// A pane's output must never become part of a hook's command line. cmd.exe
// can't escape its metacharacters, so they are dropped.
func TestQuoteStripsCmdMetacharacters(t *testing.T) {
	got := quote(`ok? & del C:\x | echo ^%PATH% "q" <>()!`)
	if want := `"ok?  del C:\x  echo PATH q "`; got != want {
		t.Errorf("quote() = %s, want %s", got, want)
	}
	if got := quote("a\r\nb"); got != `"ab"` {
		t.Errorf("newlines survived: %s", got)
	}
}
