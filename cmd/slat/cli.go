package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jxskiss/mcli"
	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/control"
)

type JSONFlag struct {
	JSON bool `cli:"--json, print one JSON object"`
}

type TargetFlags struct {
	Session   string `cli:"--session, target session name, dir, or socket"`
	Workspace string `cli:"--workspace, target workspace name or 1-based index"`
	Tab       string `cli:"--tab, target tab name or 1-based index"`
}

func registerAgentCommands(
	m *mcli.App,
	deps cliDeps,
	checkCmd func(ctx *mcli.Context, maxPositional int) bool,
	fail func(code int, err error),
	parseOpts []mcli.ParseOpt,
) {
	exec := func(req control.Request, targetSess string, asJSON bool) {
		sock := deps.defaultSocket()
		if targetSess != "" {
			resolved, err := resolveSessionSocket(sock, targetSess, deps.listSessions())
			if err != nil {
				fail(control.CodeError, err)
				return
			}
			sock = resolved
		}

		// `wait` blocks by design; the rest should answer at once.
		deadline := 15 * time.Second
		if req.Cmd == "wait" {
			deadline = 0
		}
		resp, err := deps.doControl(sock, req, deadline)
		if err != nil {
			fail(control.CodeError, err)
			return
		}
		if req.Cmd == "focus" && resp.Error == "" && targetSess != "" {
			if curSock := os.Getenv("SLAT"); curSock != "" && curSock != sock {
				_, _ = deps.doControl(curSock, control.Request{Cmd: "switch", Data: sock}, 2*time.Second)
			}
		}
		printResponse(deps.out, deps.errOut, resp, asJSON)
		deps.exit(resp.Code)
	}

	execTargeted := func(req control.Request, raw string, tf TargetFlags, asJSON bool) {
		req, sess, err := applyTarget(req, raw, tf)
		if err != nil {
			fail(control.CodeError, err)
			return
		}
		exec(req, sess, asJSON)
	}

	catAgent := mcli.WithCategory("Scripting & Agents")

	type lsArgs struct {
		JSONFlag
		Session string `cli:"--session, target session name, dir, or socket"`
	}
	addCmd(m, "ls", "list panes and what each is doing", 0, checkCmd, parseOpts,
		func(_ *mcli.Context, args *lsArgs) {
			exec(control.Request{Cmd: "ls"}, args.Session, args.JSON)
		}, catAgent)

	type statusArgs struct {
		JSONFlag
		TargetFlags
		Pane string `cli:"pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE, default active)"`
	}
	addCmd(m, "status", "report one pane's status", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *statusArgs) {
			execTargeted(control.Request{Cmd: "status"}, args.Pane, args.TargetFlags, args.JSON)
		}, catAgent)

	m.AddGroup("pane", "manage panes", catAgent)

	type paneNewArgs struct {
		JSONFlag
		TargetFlags
		Split  string `cli:"--split, v (left/right) or h (top/bottom)"`
		Cwd    string `cli:"--cwd, directory for the new shell"`
		Cmd    string `cli:"--cmd, run this command in the new pane"`
		Target string `cli:"--target, pane to split"`
		Focus  bool   `cli:"--focus, leave the new pane focused"`
	}
	addCmd(m, "pane new", "split a pane", 0, checkCmd, parseOpts,
		func(_ *mcli.Context, args *paneNewArgs) {
			req := control.Request{
				Cmd:     "pane-new",
				Split:   args.Split,
				Cwd:     args.Cwd,
				Command: args.Cmd,
				Focus:   args.Focus,
			}
			execTargeted(req, args.Target, args.TargetFlags, args.JSON)
		})

	type paneCloseArgs struct {
		JSONFlag
		TargetFlags
		Pane string `cli:"pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE, default active)"`
	}
	addCmd(m, "pane close", "close a pane", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *paneCloseArgs) {
			execTargeted(control.Request{Cmd: "pane-close"}, args.Pane, args.TargetFlags, args.JSON)
		})

	type sendArgs struct {
		JSONFlag
		TargetFlags
		Enter bool     `cli:"--enter, press Enter after the text"`
		Key   string   `cli:"--key, named key (enter, escape, ctrl-c, up, …)"`
		Pane  string   `cli:"pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE, default active)"`
		Text  []string `cli:"text, text to type into the pane"`
	}
	addCmd(m, "send", "type text or keys into a pane", -1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *sendArgs) {
			words := trimEndOfFlags(args.Text)
			text := strings.Join(words, " ")
			if args.Key != "" {
				seq, err := keySequence(args.Key)
				if err != nil {
					fail(control.CodeError, err)
					return
				}
				text += seq
			}
			if args.Enter {
				text += "\r"
			}
			if text == "" {
				fail(control.CodeError, fmt.Errorf("send needs text, --key or --enter"))
				return
			}
			execTargeted(control.Request{Cmd: "send", Data: text}, args.Pane, args.TargetFlags, args.JSON)
		}, catAgent)

	type runArgs struct {
		JSONFlag
		TargetFlags
		Pane    string   `cli:"#R, pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE)"`
		Command []string `cli:"#R, command, command to send and run"`
	}
	addCmd(m, "run", "send a command and press Enter", -1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *runArgs) {
			words := trimEndOfFlags(args.Command)
			if len(words) == 0 {
				fail(control.CodeError, fmt.Errorf("run needs a command"))
				return
			}
			req := control.Request{Cmd: "send", Data: strings.Join(words, " ") + "\r"}
			execTargeted(req, args.Pane, args.TargetFlags, args.JSON)
		}, catAgent)

	type focusArgs struct {
		JSONFlag
		TargetFlags
		Target string `cli:"target, target pane/workspace/tab or direction (left, down, up, right)"`
	}
	addCmd(m, "focus", "move focus to a pane or direction", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *focusArgs) {
			req := control.Request{Cmd: "focus"}
			if args.Workspace == "" && args.Tab == "" &&
				(args.Target == "left" || args.Target == "down" || args.Target == "up" || args.Target == "right") {
				req.Direction = args.Target
				exec(req, args.Session, args.JSON)
				return
			}
			sess, ws, tab, paneSpec, err := parseTarget(args.Target, args.Session, args.Workspace, args.Tab)
			if err != nil {
				fail(control.CodeError, err)
				return
			}
			if paneSpec == "" && sess == "" && ws == "" && tab == "" {
				fail(control.CodeError, fmt.Errorf("focus needs a target or direction (left, down, up, right)"))
				return
			}
			if paneSpec == "" {
				paneSpec = "active"
			}
			req.Pane = paneSpec
			req.TargetWorkspace = ws
			req.TargetTab = tab
			exec(req, sess, args.JSON)
		}, catAgent)

	type displayArgs struct {
		JSONFlag
		Session  string   `cli:"--session, target session name, dir, or socket"`
		Duration string   `cli:"--duration, how long to show the message (default 2s)"`
		Message  []string `cli:"#R, message, message to display in the status bar"`
	}
	addCmd(m, "display", "show an alert in the status bar", -1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *displayArgs) {
			words := trimEndOfFlags(args.Message)
			if len(words) == 0 {
				fail(control.CodeError, fmt.Errorf("display needs a message"))
				return
			}
			req := control.Request{
				Cmd:     "display",
				Data:    strings.Join(words, " "),
				Timeout: args.Duration,
			}
			exec(req, args.Session, args.JSON)
		}, catAgent)

	type captureArgs struct {
		JSONFlag
		TargetFlags
		Lines   int    `cli:"--lines, only the last N lines"`
		History bool   `cli:"--history, include the scrollback"`
		Pane    string `cli:"pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE, default active)"`
	}
	addCmd(m, "capture", "print a pane's text", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *captureArgs) {
			req := control.Request{
				Cmd:     "capture",
				Lines:   args.Lines,
				History: args.History,
			}
			execTargeted(req, args.Pane, args.TargetFlags, args.JSON)
		}, catAgent)

	type waitArgs struct {
		JSONFlag
		TargetFlags
		For     string `cli:"#R, --for, condition: idle, input, exit, or text=REGEX"`
		Timeout string `cli:"--timeout, give up after this long (default 60s)"`
		Pane    string `cli:"pane, target pane ([SESSION:][WORKSPACE/[TAB/]]PANE, default active)"`
	}
	addCmd(m, "wait", "block until a condition holds", 1, checkCmd, parseOpts,
		func(_ *mcli.Context, args *waitArgs) {
			if _, err := app.ParseWaitFor(args.For); err != nil {
				fail(control.CodeError, err)
				return
			}
			req := control.Request{
				Cmd:     "wait",
				For:     args.For,
				Timeout: args.Timeout,
			}
			execTargeted(req, args.Pane, args.TargetFlags, args.JSON)
		}, catAgent)
}

func trimEndOfFlags(words []string) []string {
	if len(words) > 0 && words[0] == "--" {
		return words[1:]
	}
	return words
}

func applyTarget(req control.Request, raw string, tf TargetFlags) (control.Request, string, error) {
	sess, ws, tab, paneSpec, err := parseTarget(raw, tf.Session, tf.Workspace, tf.Tab)
	if err != nil {
		return req, "", err
	}
	req.Pane = paneSpec
	req.TargetWorkspace = ws
	req.TargetTab = tab
	return req, sess, nil
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

// printResponse writes the response: one JSON object, or a short human line.
func printResponse(out, errOut io.Writer, resp control.Response, asJSON bool) {
	if asJSON {
		b, _ := json.Marshal(resp)
		fmt.Fprintln(out, string(b))
		return
	}
	if resp.Error != "" {
		fmt.Fprintf(errOut, "slat: %s\n", resp.Error)
	}
	switch {
	case resp.Lines != nil:
		fmt.Fprintln(out, strings.Join(resp.Lines, "\n"))
	case resp.Panes != nil:
		for _, p := range resp.Panes {
			fmt.Fprintln(out, paneLine(p))
		}
	case resp.Pane != nil:
		fmt.Fprintln(out, paneLine(*resp.Pane))
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
