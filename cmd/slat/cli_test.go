package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/control"
)

type cliTestResult struct {
	calledControl bool
	calledClient  bool
	sock          string
	req           control.Request
	exitCode      int
	out           string
	errOut        string
}

func runTestCLI(args ...string) cliTestResult {
	var res cliTestResult
	var outBuf, errBuf bytes.Buffer
	deps := defaultDeps()
	deps.continueOnErr = true
	deps.out = &outBuf
	deps.errOut = &errBuf
	deps.defaultSocket = func() string { return "/default.sock" }
	deps.listSessions = func() []app.SessionSummary {
		return []app.SessionSummary{
			{Name: "work", Dir: "/tmp/work", Socket: "/tmp/slat-work.sock"},
		}
	}
	deps.exit = func(code int) { res.exitCode = code }
	deps.runClient = func(sock, dir, presetPath, configPath string) error {
		res.calledClient = true
		return nil
	}
	deps.doControl = func(sock string, req control.Request, timeout time.Duration) (control.Response, error) {
		res.calledControl = true
		res.sock = sock
		res.req = req
		return control.Response{Schema: control.Schema, Code: control.CodeOK}, nil
	}
	m, _ := newCLIApp(deps)
	m.Run(args...)
	res.out = outBuf.String()
	res.errOut = errBuf.String()
	return res
}

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
		{[]string{"send", "--enter", "2", "--", "ls"}, "send 2 ls\r"},
		{[]string{"send", "2", "--", "--enter"}, "send 2 --enter"},
		{[]string{"run", "2", "--", "ls", "-la"}, "send 2 ls -la\r"},
		{[]string{"send", "2", "--key", "ctrl-c"}, "send 2 \x03"},
		{[]string{"send", "2", "--key", "escape"}, "send 2 \x1b"},
		{[]string{"wait", "2", "--for", "idle"}, "wait 2 idle"},
		{[]string{"wait", "--for", "text=done"}, "wait  text=done"},

		// Errors, because an agent must not silently get a different action.
		{[]string{"send", "2"}, ""},
		{[]string{"run", "2"}, ""},
		{[]string{"wait", "2"}, ""},
		{[]string{"wait", "2", "--for", "done"}, ""},
		{[]string{"wait", "2", "--for", "text=("}, ""},
		{[]string{"send", "2", "--key", "meta-x"}, ""},
		{[]string{"ls", "extra"}, ""},
	}
	for _, c := range cases {
		res := runTestCLI(c.args...)
		if c.want == "" {
			if res.calledControl {
				t.Errorf("%v: expected no control call, got req=%+v", c.args, res.req)
			}
			continue
		}
		if res.exitCode != 0 || !res.calledControl {
			t.Errorf("%v: exit=%d err=%s", c.args, res.exitCode, res.errOut)
			continue
		}
		got := res.req.Cmd + " " + res.req.Pane + " " + res.req.Data + res.req.For
		if got != c.want {
			t.Errorf("%v: got %q, want %q", c.args, got, c.want)
		}
	}
}

// Only the agent verbs are control-socket commands; root/version/attach/unknown
// must not invoke doControl.
func TestRunCLIIgnoresOtherArgs(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"attach"}, {"-x"}, {"unknown-cmd"}, {"pane"}} {
		res := runTestCLI(args...)
		if res.calledControl {
			t.Errorf("%v unexpectedly invoked control socket", args)
		}
	}
	if res := runTestCLI("-x"); res.exitCode == 0 {
		t.Errorf("expected non-zero exit for -x")
	}
}

func TestParseCLIWithTargeting(t *testing.T) {
	cases := []struct {
		args     []string
		wantSock string
		wantWS   string
		wantTab  string
		wantPane string
		wantErr  bool
	}{
		{args: []string{"send", "work:2", "hi"}, wantSock: "/tmp/slat-work.sock", wantPane: "2"},
		{args: []string{"send", "work:dev/active", "hi"}, wantSock: "/tmp/slat-work.sock", wantWS: "dev", wantPane: "active"},
		{args: []string{"send", "work:dev/editor/5", "hi"}, wantSock: "/tmp/slat-work.sock", wantWS: "dev", wantTab: "editor", wantPane: "5"},
		{args: []string{"send", "--session", "work", "--workspace", "dev", "--tab", "2", "active", "hi"}, wantSock: "/tmp/slat-work.sock", wantWS: "dev", wantTab: "2", wantPane: "active"},
		{args: []string{"send", "--session", "a", "b:2", "hi"}, wantErr: true},
		{args: []string{"send", ":2", "hi"}, wantErr: true},
		{args: []string{"send", "a/b/c/d", "hi"}, wantErr: true},
	}
	for _, tc := range cases {
		res := runTestCLI(tc.args...)
		if tc.wantErr {
			if res.exitCode == 0 || res.calledControl {
				t.Errorf("%v: expected error, got %+v", tc.args, res.req)
			}
			continue
		}
		if res.exitCode != 0 || !res.calledControl {
			t.Errorf("%v: unexpected err: exit=%d err=%s", tc.args, res.exitCode, res.errOut)
			continue
		}
		if res.sock != tc.wantSock || res.req.TargetWorkspace != tc.wantWS || res.req.TargetTab != tc.wantTab || res.req.Pane != tc.wantPane {
			t.Errorf("%v: got (%q, %q, %q, %q), want (%q, %q, %q, %q)",
				tc.args, res.sock, res.req.TargetWorkspace, res.req.TargetTab, res.req.Pane,
				tc.wantSock, tc.wantWS, tc.wantTab, tc.wantPane)
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

func TestParseCLIFocusDirection(t *testing.T) {
	t.Setenv("SLAT", "")
	res := runTestCLI("focus", "--session", "work", "up")
	if res.exitCode != 0 || res.req.Cmd != "focus" || res.req.Direction != "up" || res.sock != "/tmp/slat-work.sock" {
		t.Fatalf("focus up = (%+v, %q, exit=%d, err=%s)", res.req, res.sock, res.exitCode, res.errOut)
	}
	res2 := runTestCLI("focus", "work:dev/editor/3")
	if res2.exitCode != 0 || res2.sock != "/tmp/slat-work.sock" || res2.req.TargetWorkspace != "dev" || res2.req.TargetTab != "editor" || res2.req.Pane != "3" {
		t.Fatalf("focus inline target = (%+v, %q, exit=%d)", res2.req, res2.sock, res2.exitCode)
	}
	res3 := runTestCLI("focus", "--workspace", "dev", "--tab", "2")
	if res3.exitCode != 0 || res3.req.TargetWorkspace != "dev" || res3.req.TargetTab != "2" || res3.req.Pane != "active" {
		t.Fatalf("focus flags only = (%+v, exit=%d)", res3.req, res3.exitCode)
	}
	res4 := runTestCLI("focus", "2")
	if res4.exitCode != 0 || res4.req.Pane != "2" || res4.req.Direction != "" {
		t.Fatalf("focus pane id = (%+v, exit=%d)", res4.req, res4.exitCode)
	}
	if r := runTestCLI("focus"); r.exitCode == 0 || r.calledControl {
		t.Errorf("expected error for empty focus")
	}
	if r := runTestCLI("focus", "2", "extra"); r.exitCode == 0 || r.calledControl {
		t.Errorf("expected error for extra focus arg")
	}
}

func TestParseCLIDisplay(t *testing.T) {
	res := runTestCLI("display", "--duration", "5s", "--session", "work", "hello", "world")
	if res.exitCode != 0 {
		t.Fatalf("display parse err: exit=%d err=%s", res.exitCode, res.errOut)
	}
	if res.req.Cmd != "display" || res.req.Data != "hello world" || res.req.Timeout != "5s" || res.sock != "/tmp/slat-work.sock" {
		t.Errorf("display got (%+v, %q)", res.req, res.sock)
	}
	if r := runTestCLI("display"); r.exitCode == 0 || r.calledControl {
		t.Errorf("expected error for empty display message")
	}
}
