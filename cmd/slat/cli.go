package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/noturbob/slat/internal/app"
	"github.com/noturbob/slat/internal/control"
)

const cliUsage = `slat commands (for scripts and agents):

  slat ls [--json]                       list panes and what each is doing
  slat status [PANE] [--json]            one pane's status
  slat pane new [--split v|h] [--cwd D] [--cmd C] [--target P] [--focus]
  slat pane close PANE
  slat send PANE TEXT... [--enter] [--key KEY]
  slat run PANE COMMAND...               send COMMAND and press Enter
  slat capture PANE [--lines N] [--history]
  slat wait PANE --for idle|input|exit|text=REGEX [--timeout 60s]

PANE is a pane id from 'slat ls', or 'active' (the default).
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
	case "ls", "status", "pane", "send", "run", "capture", "wait":
	default:
		return false, 0
	}

	req, asJSON, err := parseCLI(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "slat: %v\n", err)
		return true, control.CodeError
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
	print(resp, req, asJSON)
	return true, resp.Code
}

func parseCLI(args []string) (control.Request, bool, error) {
	cmd, rest := args[0], args[1:]
	req := control.Request{Cmd: cmd}

	// `slat pane new|close` is two words.
	if cmd == "pane" {
		if len(rest) == 0 {
			return req, false, fmt.Errorf("pane needs new or close (slat --help)")
		}
		switch rest[0] {
		case "new", "close":
			req.Cmd, rest = "pane-"+rest[0], rest[1:]
		default:
			return req, false, fmt.Errorf("pane %q: expected new or close", rest[0])
		}
	}

	fs := flag.NewFlagSet("slat "+cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asJSON := fs.Bool("json", false, "print one JSON object")
	enter := fs.Bool("enter", false, "send: press Enter after the text")
	key := fs.String("key", "", "send: a named key (enter, escape, ctrl-c, up, …)")
	fs.StringVar(&req.Split, "split", "", "pane new: v (left/right) or h (top/bottom)")
	fs.StringVar(&req.Cwd, "cwd", "", "pane new: directory for the new shell")
	fs.StringVar(&req.Command, "cmd", "", "pane new: run this command in it")
	fs.StringVar(&req.Pane, "target", "", "pane new: the pane to split")
	fs.BoolVar(&req.Focus, "focus", false, "pane new: leave the new pane focused")
	fs.IntVar(&req.Lines, "lines", 0, "capture: only the last N lines")
	fs.BoolVar(&req.History, "history", false, "capture: include the scrollback")
	fs.StringVar(&req.For, "for", "", "wait: idle, input, exit or text=REGEX")
	fs.StringVar(&req.Timeout, "timeout", "", "wait: give up after this long (default 60s)")
	flags, words := permute(rest)
	if err := fs.Parse(flags); err != nil {
		return req, false, err
	}

	// The first bare word is the pane for every command that takes one.
	takesPane := req.Cmd != "ls" && req.Cmd != "pane-new"
	if takesPane && len(words) > 0 {
		req.Pane, words = words[0], words[1:]
	}

	switch req.Cmd {
	case "send":
		text := strings.Join(words, " ")
		if *key != "" {
			seq, err := keySequence(*key)
			if err != nil {
				return req, false, err
			}
			text += seq
		}
		if *enter {
			text += "\r"
		}
		if text == "" {
			return req, false, fmt.Errorf("send needs text, --key or --enter")
		}
		req.Data = text
	case "run":
		if len(words) == 0 {
			return req, false, fmt.Errorf("run needs a command")
		}
		req.Cmd, req.Data = "send", strings.Join(words, " ")+"\r"
	case "wait":
		if req.For == "" {
			return req, false, fmt.Errorf("wait needs --for idle|input|exit|text=REGEX")
		}
		if _, err := app.ParseWaitFor(req.For); err != nil {
			return req, false, err
		}
	}
	if len(words) > 0 && req.Cmd != "send" {
		return req, false, fmt.Errorf("unexpected argument %q", words[0])
	}
	return req, *asJSON, nil
}

// valueFlags take a following argument, so permute knows that argument
// isn't a bare word.
var valueFlags = map[string]bool{
	"split": true, "cwd": true, "cmd": true, "target": true,
	"lines": true, "for": true, "timeout": true, "key": true,
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
