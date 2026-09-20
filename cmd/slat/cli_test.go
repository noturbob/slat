package main

import "testing"

func TestParseCLI(t *testing.T) {
	cases := []struct {
		args []string
		want string // "cmd pane data|for" of the request, or "" for an error
	}{
		{[]string{"ls"}, "ls  "},
		{[]string{"status", "3"}, "status 3 "},
		{[]string{"pane", "new", "--split", "h"}, "pane-new  "},
		{[]string{"pane", "close", "2"}, "pane-close 2 "},
		{[]string{"run", "2", "go", "build", "./..."}, "send 2 go build ./...\r"},
		{[]string{"send", "2", "y", "--enter"}, "send 2 y\r"},
		{[]string{"send", "2", "--key", "ctrl-c"}, "send 2 \x03"},
		{[]string{"send", "2", "--key", "escape"}, "send 2 \x1b"},
		{[]string{"wait", "2", "--for", "idle"}, "wait 2 idle"},
		{[]string{"wait", "--for", "text=done"}, "wait  text=done"},

		// Errors, because an agent must not silently get a different action.
		{[]string{"pane"}, ""},
		{[]string{"pane", "split"}, ""},
		{[]string{"send", "2"}, ""},
		{[]string{"run", "2"}, ""},
		{[]string{"wait", "2"}, ""},
		{[]string{"wait", "2", "--for", "done"}, ""},
		{[]string{"wait", "2", "--for", "text=("}, ""},
		{[]string{"send", "2", "--key", "meta-x"}, ""},
		{[]string{"ls", "extra"}, ""},
	}
	for _, c := range cases {
		req, _, err := parseCLI(c.args)
		if c.want == "" {
			if err == nil {
				t.Errorf("%v: expected an error, got %+v", c.args, req)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		got := req.Cmd + " " + req.Pane + " " + req.Data + req.For
		if got != c.want {
			t.Errorf("%v: got %q, want %q", c.args, got, c.want)
		}
	}
}

// Only the agent verbs are CLI commands; anything else must fall through
// to the attach path (or the unknown-argument error), not be swallowed.
func TestRunCLIIgnoresOtherArgs(t *testing.T) {
	for _, args := range [][]string{{}, {"--version"}, {"attach"}, {"-x"}} {
		if handled, _ := runCLI("/nonexistent.sock", args); handled {
			t.Errorf("%v was treated as a CLI command", args)
		}
	}
}
