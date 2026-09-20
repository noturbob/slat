//go:build !windows

package app

import "testing"

func TestQuoteEscapesSingleQuotes(t *testing.T) {
	if got, want := quote(`it's; rm -rf /`), `'it'\''s; rm -rf /'`; got != want {
		t.Errorf("quote() = %s, want %s", got, want)
	}
}
