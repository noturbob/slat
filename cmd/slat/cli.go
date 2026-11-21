package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/control"
)

const cliUsage = `slat commands (for scripts and agents):

  slat sessions [--json]                 list running sessions
  slat attach [NAME|DIR]                 attach to a session by name or directory
  slat session NAME|PATH                 start or attach to a preconfigured session
  slat ls [--json]                       list panes and what each is doing
  slat status [PANE] [--json]            one pane's status
  slat pane new [--split v|h] [--cwd D] [--cmd C] [--target P] [--focus]
  slat pane close PANE
  slat send PANE TEXT... [--enter] [--key KEY]
  slat run PANE COMMAND...               send COMMAND and press Enter
  slat focus TARGET|left|down|up|right   move focus to a pane or direction
  slat display MESSAGE [--duration 2s]   show an alert in the status bar
  slat capture PANE [--lines N] [--history]
  slat wait PANE --for idle|input|exit|text=REGEX [--timeout 60s]

PANE is a pane id from 'slat ls', 'active' (the default), or a scoped
target: [SESSION:][WORKSPACE/[TAB/]]PANE (e.g. work:2, dev/editor/active).
Flags --session, --workspace, and --tab may also be used.
--json prints one JSON object; exit codes: 0 ok, 1 error, 2 timeout,
3 the pane's program has exited.

Keys for --key: enter, tab, escape, space, up, down, left, right,
backspace, delete, home, end, pageup, pagedown, f1-f12, ctrl-X, alt-X.
`

// runCLI handles the agent-facing subcommands. It reports whether args
// were a CLI command at all; if not, main falls through to attaching.
func runCLI(sock string, args []string) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "ls", "status", "pane", "send", "run", "capture", "wait", "focus", "display":
	default:
		return false, 0
	}

	req, targetSess, asJSON, err := parseCLIWithSession(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		return true, control.CodeError
	}
	if targetSess != "" {
		resolved, err := resolveSessionSocket(sock, targetSess, listSessions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "slat: %v\n", err)
			return true, control.CodeError
		}
		sock = resolved
	}

	// `wait` blocks by design; the rest should answer at once.
	deadline := 15 * time.Second
	if req.Cmd == "wait" {
		deadline = 0
	}
	resp, err := control.Do(sock, req, deadline)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		return true, control.CodeError
	}
	if req.Cmd == "focus" && resp.Error == "" && targetSess != "" {
		if curSock := os.Getenv("SLAT"); curSock != "" && curSock != sock {
			_, _ = control.Do(curSock, control.Request{Cmd: "switch", Data: sock}, 2*time.Second)
		}
	}
	print(resp, req, asJSON)
	return true, resp.Code
}

func parseCLI(args []string) (control.Request, bool, error) {
	req, _, asJSON, err := parseCLIWithSession(args)
	return req, asJSON, err
}

func parseCLIWithSession(args []string) (control.Request, string, bool, error) {
	cmd, rest := args[0], args[1:]
	req := control.Request{Cmd: cmd}

	// `slat pane new|close` is two words.
	if cmd == "pane" {
		if len(rest) == 0 {
			return req, "", false, fmt.Errorf("pane needs new or close (slat --help)")
		}
		switch rest[0] {
		case "new", "close":
			req.Cmd, rest = "pane-"+rest[0], rest[1:]
		default:
			return req, "", false, fmt.Errorf("pane %q: expected new or close", rest[0])
		}
	}

	fs := flag.NewFlagSet("slat "+cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print one JSON object")
	enter := fs.Bool("enter", false, "send: press Enter after the text")
	key := fs.String("key", "", "send: a named key (enter, escape, ctrl-c, up, …)")
	flagSession := fs.String("session", "", "target session name, dir, or socket")
	flagWS := fs.String("workspace", "", "target workspace name or 1-based index")
	flagTab := fs.String("tab", "", "target tab name or 1-based index")
	fs.StringVar(&req.Split, "split", "", "pane new: v (left/right) or h (top/bottom)")
	fs.StringVar(&req.Cwd, "cwd", "", "pane new: directory for the new shell")
	fs.StringVar(&req.Command, "cmd", "", "pane new: run this command in it")
	fs.StringVar(&req.Pane, "target", "", "pane new: the pane to split")
	fs.BoolVar(&req.Focus, "focus", false, "pane new: leave the new pane focused")
	fs.IntVar(&req.Lines, "lines", 0, "capture: only the last N lines")
	fs.BoolVar(&req.History, "history", false, "capture: include the scrollback")
	fs.StringVar(&req.For, "for", "", "wait: idle, input, exit or text=REGEX")
	fs.StringVar(&req.Timeout, "timeout", "", "wait: give up after this long (default 60s)")
	flagDuration := fs.String("duration", "", "display: how long to show the message (default 2s)")
	flags, words := permute(rest)
	if err := fs.Parse(flags); err != nil {
		return req, "", false, err
	}

	// The first bare word is the pane for every command that takes one.
	takesPane := req.Cmd != "ls" && req.Cmd != "pane-new" && req.Cmd != "focus" && req.Cmd != "display"
	if takesPane && len(words) > 0 {
		req.Pane, words = words[0], words[1:]
	}

	if req.Cmd == "display" {
		if len(words) == 0 {
			return req, "", false, fmt.Errorf("display needs a message")
		}
		req.Data = strings.Join(words, " ")
		if *flagDuration != "" {
			req.Timeout = *flagDuration
		}
		return req, *flagSession, *asJSON, nil
	}

	if req.Cmd == "focus" {
		if len(words) > 1 {
			return req, "", false, fmt.Errorf("focus takes at most one target or direction")
		}
		raw := ""
		if len(words) == 1 {
			raw = words[0]
		}
		if *flagWS == "" && *flagTab == "" && (raw == "left" || raw == "down" || raw == "up" || raw == "right") {
			req.Direction = raw
			return req, *flagSession, *asJSON, nil
		}
		sess, ws, tab, paneSpec, err := parseTarget(raw, *flagSession, *flagWS, *flagTab)
		if err != nil {
			return req, "", false, err
		}
		if paneSpec == "" && sess == "" && ws == "" && tab == "" {
			return req, "", false, fmt.Errorf("focus needs a target or direction (left, down, up, right)")
		}
		if paneSpec == "" {
			paneSpec = "active"
		}
		req.Pane = paneSpec
		req.TargetWorkspace = ws
		req.TargetTab = tab
		return req, sess, *asJSON, nil
	}

	sess, ws, tab, paneSpec, err := parseTarget(req.Pane, *flagSession, *flagWS, *flagTab)
	if err != nil {
		return req, "", false, err
	}
	req.Pane = paneSpec
	req.TargetWorkspace = ws
	req.TargetTab = tab

	switch req.Cmd {
	case "send":
		text := strings.Join(words, " ")
		if *key != "" {
			seq, err := keySequence(*key)
			if err != nil {
				return req, "", false, err
			}
			text += seq
		}
		if *enter {
			text += "\r"
		}
		if text == "" {
			return req, "", false, fmt.Errorf("send needs text, --key or --enter")
		}
		req.Data = text
	case "run":
		if len(words) == 0 {
			return req, "", false, fmt.Errorf("run needs a command")
		}
		req.Cmd, req.Data = "send", strings.Join(words, " ")+"\r"
	case "wait":
		if req.For == "" {
			return req, "", false, fmt.Errorf("wait needs --for idle|input|exit|text=REGEX")
		}
		if _, err := app.ParseWaitFor(req.For); err != nil {
			return req, "", false, err
		}
	}
	if len(words) > 0 && req.Cmd != "send" {
		return req, "", false, fmt.Errorf("unexpected argument %q", words[0])
	}
	return req, sess, *asJSON, nil
}

func parseTarget(raw, flagSession, flagWS, flagTab string) (sess, ws, tab, pane string, err error) {
	var inlineSess, inlineWS, inlineTab string
	rest := raw
	if colon := strings.IndexByte(rest, ':'); colon >= 0 {
		inlineSess = rest[:colon]
		rest = rest[colon+1:]
		if inlineSess == "" {
			return "", "", "", "", fmt.Errorf("empty session in target %q", raw)
		}
	}
	if rest != "" {
		parts := strings.Split(rest, "/")
		switch len(parts) {
		case 1:
			pane = parts[0]
		case 2:
			if parts[0] == "" {
				return "", "", "", "", fmt.Errorf("empty workspace in target %q", raw)
			}
			inlineWS, pane = parts[0], parts[1]
		case 3:
			if parts[0] == "" || parts[1] == "" {
				return "", "", "", "", fmt.Errorf("empty workspace or tab in target %q", raw)
			}
			inlineWS, inlineTab, pane = parts[0], parts[1], parts[2]
		default:
			return "", "", "", "", fmt.Errorf("invalid target %q", raw)
		}
	}

	if sess, err = mergeTargetField("session", inlineSess, flagSession); err != nil {
		return "", "", "", "", err
	}
	if ws, err = mergeTargetField("workspace", inlineWS, flagWS); err != nil {
		return "", "", "", "", err
	}
	if tab, err = mergeTargetField("tab", inlineTab, flagTab); err != nil {
		return "", "", "", "", err
	}
	return sess, ws, tab, pane, nil
}

func mergeTargetField(kind, inline, flagVal string) (string, error) {
	if inline != "" && flagVal != "" && inline != flagVal {
		return "", fmt.Errorf("conflicting %s: %q vs %q", kind, inline, flagVal)
	}
	if flagVal != "" {
		return flagVal, nil
	}
	return inline, nil
}

func resolveSessionSocket(defaultSock, spec string, sessions []app.SessionSummary) (string, error) {
	if spec == "" {
		return defaultSock, nil
	}
	if fi, err := os.Stat(spec); err == nil && fi.Mode()&os.ModeSocket != 0 {
		return spec, nil
	}
	var matches []app.SessionSummary
	for _, s := range sessions {
		if s.Name == spec || filepath.Base(s.Socket) == spec || s.Dir == spec {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session %q", spec)
	case 1:
		return matches[0].Socket, nil
	default:
		return "", fmt.Errorf("ambiguous session %q (%d matches); use socket path", spec, len(matches))
	}
}

// valueFlags take a following argument, so permute knows that argument
// isn't a bare word.
var valueFlags = map[string]bool{
	"split": true, "cwd": true, "cmd": true, "target": true,
	"lines": true, "for": true, "timeout": true, "duration": true, "key": true,
	"session": true, "workspace": true, "tab": true,
}

// permute separates flags from bare words anywhere in args. Go's flag
// package stops parsing at the first bare word, but `slat wait 2 --for
// idle` is how people and agents write it. Everything after `--` is a
// word, so text may start with a dash.
func permute(args []string) (flags, words []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return flags, append(words, args[i+1:]...)
		case len(a) > 1 && a[0] == '-':
			flags = append(flags, a)
			if name := strings.TrimLeft(a, "-"); valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			words = append(words, a)
		}
	}
	return flags, words
}

// print writes the response: one JSON object, or a short human line.
func print(resp control.Response, req control.Request, asJSON bool) {
	if asJSON {
		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
		if resp.Error != "" {
			return
		}
		return
	}
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "slat: %s\n", resp.Error)
	}
	switch {
	case resp.Lines != nil:
		fmt.Println(strings.Join(resp.Lines, "\n"))
	case resp.Panes != nil:
		for _, p := range resp.Panes {
			fmt.Println(paneLine(p))
		}
	case resp.Pane != nil:
		fmt.Println(paneLine(*resp.Pane))
	}
}

func paneLine(p app.PaneInfo) string {
	mark := " "
	if p.Active {
		mark = "*"
	}
	line := fmt.Sprintf("%s%-3d %-8s %d:%-10s %-10s", mark, p.Pane, p.Status, p.Tab, p.TabName, p.Foreground)
	if p.Cwd != "" {
		line += " " + p.Cwd
	}
	return strings.TrimRight(line, " ")
}

// keySequence turns a key name into the bytes a terminal sends for it.
func keySequence(name string) (string, error) {
	named := map[string]string{
		"enter": "\r", "return": "\r", "tab": "\t", "escape": "\x1b", "esc": "\x1b",
		"space": " ", "backspace": "\x7f", "delete": "\x1b[3~",
		"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
		"home": "\x1b[H", "end": "\x1b[F", "pageup": "\x1b[5~", "pagedown": "\x1b[6~",
		"f1": "\x1bOP", "f2": "\x1bOQ", "f3": "\x1bOR", "f4": "\x1bOS",
		"f5": "\x1b[15~", "f6": "\x1b[17~", "f7": "\x1b[18~", "f8": "\x1b[19~",
		"f9": "\x1b[20~", "f10": "\x1b[21~", "f11": "\x1b[23~", "f12": "\x1b[24~",
	}
	key := strings.ToLower(name)
	if seq, ok := named[key]; ok {
		return seq, nil
	}
	if after, ok := strings.CutPrefix(key, "ctrl-"); ok && len(after) == 1 {
		c := after[0]
		if c >= 'a' && c <= 'z' {
			return string(rune(c & 0x1f)), nil
		}
	}
	if after, ok := strings.CutPrefix(key, "alt-"); ok && len(after) == 1 {
		return "\x1b" + after, nil
	}
	return "", fmt.Errorf("unknown key %q", name)
}

func runSessions(args []string) int {
	fs := flag.NewFlagSet("slat sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print one JSON object")
	if err := fs.Parse(args); err != nil {
		return control.CodeError
	}
	sessions := listSessions()
	if *asJSON {
		b, _ := json.Marshal(control.Response{Schema: control.Schema, Sessions: sessions})
		fmt.Println(string(b))
		return control.CodeOK
	}
	if len(sessions) == 0 {
		fmt.Println("no active sessions")
		return control.CodeOK
	}
	for _, s := range sessions {
		state := "detached"
		if s.Attached {
			state = "attached"
		}
		fmt.Printf("%-14s  %d tabs  %d panes  %-8s  %s\n", s.Name, s.Tabs, s.Panes, state, s.Dir)
	}
	return control.CodeOK
}
