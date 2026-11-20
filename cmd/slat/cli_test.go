package main

import (
	"testing"

	"github.com/noturbob/slat/internal/app"
)

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

func TestParseCLIWithTargeting(t *testing.T) {
	cases := []struct {
		args     []string
		wantSess string
		wantWS   string
		wantTab  string
		wantPane string
		wantErr  bool
	}{
		{args: []string{"send", "work:2", "hi"}, wantSess: "work", wantPane: "2"},
		{args: []string{"send", "work:dev/active", "hi"}, wantSess: "work", wantWS: "dev", wantPane: "active"},
		{args: []string{"send", "work:dev/editor/5", "hi"}, wantSess: "work", wantWS: "dev", wantTab: "editor", wantPane: "5"},
		{args: []string{"send", "--session", "work", "--workspace", "dev", "--tab", "2", "active", "hi"}, wantSess: "work", wantWS: "dev", wantTab: "2", wantPane: "active"},
		{args: []string{"send", "--session", "a", "b:2", "hi"}, wantErr: true},
		{args: []string{"send", ":2", "hi"}, wantErr: true},
		{args: []string{"send", "a/b/c/d", "hi"}, wantErr: true},
	}
	for _, tc := range cases {
		req, sess, _, err := parseCLIWithSession(tc.args)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%v: expected error, got %+v", tc.args, req)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: unexpected err: %v", tc.args, err)
			continue
		}
		if sess != tc.wantSess || req.TargetWorkspace != tc.wantWS || req.TargetTab != tc.wantTab || req.Pane != tc.wantPane {
			t.Errorf("%v: got (%q, %q, %q, %q), want (%q, %q, %q, %q)",
				tc.args, sess, req.TargetWorkspace, req.TargetTab, req.Pane,
				tc.wantSess, tc.wantWS, tc.wantTab, tc.wantPane)
		}
	}
}

func TestResolveSessionSocket(t *testing.T) {
	sessions := []app.SessionSummary{
		{Name: "alpha", Dir: "/tmp/a", Socket: "/tmp/slat-a.sock"},
		{Name: "beta", Dir: "/tmp/b", Socket: "/tmp/slat-b.sock"},
		{Name: "dup", Dir: "/tmp/d1", Socket: "/tmp/slat-d1.sock"},
		{Name: "dup", Dir: "/tmp/d2", Socket: "/tmp/slat-d2.sock"},
	}
	if got, err := resolveSessionSocket("/default.sock", "", sessions); err != nil || got != "/default.sock" {
		t.Errorf("empty spec = (%q, %v)", got, err)
	}
	if got, err := resolveSessionSocket("/default.sock", "beta", sessions); err != nil || got != "/tmp/slat-b.sock" {
		t.Errorf("by name = (%q, %v)", got, err)
	}
	if _, err := resolveSessionSocket("/default.sock", "missing", sessions); err == nil {
		t.Errorf("expected error for missing session")
	}
	if _, err := resolveSessionSocket("/default.sock", "dup", sessions); err == nil {
		t.Errorf("expected error for ambiguous session")
	}
}
