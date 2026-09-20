# Agent control CLI — design

Status: draft for review · target: slat 1.0

## Why

Coding agents (Claude Code, Codex, Gemini CLI, Grok, and whatever comes
next) drive computers by **running shell commands**. They already use
terminal multiplexers to keep long-running work alive, by scripting
`tmux send-keys` and `tmux capture-pane` and guessing when a command has
finished. That guessing is the weak point: an agent cannot tell a pane
that is compiling from one that has been sitting on a `[y/N]` prompt for
an hour.

slat can answer that question exactly, because every pane already has its
own terminal emulator: slat knows each pane's screen contents, its cursor,
its scrollback and whether its program is running.

So: expose the session over the socket the daemon already has, as
ordinary commands with machine-readable output. No API keys, no network,
no vendor integrations — an agent that can run `slat` can drive slat.

## Shape

One binary, subcommands, `--json` everywhere. Running `slat` with no
arguments still just attaches, as today.

```
slat ls                                  list tabs and panes
slat pane new [--split v|h] [--cwd DIR] [--cmd CMD] [--tab N]
slat pane close <pane>
slat send <pane> <text>... [--enter] [--key KEY]
slat run <pane> <command>...             send + Enter (convenience)
slat capture <pane> [--lines N] [--history] [--ansi]
slat wait <pane> --for idle|input|exit|text=REGEX [--timeout 60s]
slat status [<pane>]                     per-pane state
```

`<pane>` is a pane id from `slat ls` (`1`, `2`, …), or `active`.

Everything prints a short human-readable line by default and a JSON
object with `--json`. Agents pass `--json`; humans reading their agent's
transcript can still follow along.

### Exit codes

Agents branch on exit codes before parsing anything:

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | error (bad arguments, no daemon, unknown pane) |
| 2 | `wait` timed out |
| 3 | the pane is gone (its program exited) |

### JSON

One object per command, never a stream, with a `schema` field so future
changes are detectable:

```json
{"schema": 1, "pane": 2, "tab": 1, "status": "idle", "cwd": "/home/me/api",
 "command": "go test ./...", "exit_code": 0}
```

`slat ls --json` returns `{"schema":1,"panes":[…]}`. Errors return
`{"schema":1,"error":"no pane 7"}` on stderr with a non-zero exit code.

## Pane status

The heart of it. Four values:

| Status | Meaning | How slat decides |
|---|---|---|
| `working` | a program is running | the pane's foreground process group is not the shell, or the pane produced output recently |
| `idle` | the shell is waiting | foreground process is the shell **and** no output for `settle` (default 750ms) |
| `input` | something is waiting for a human | `working`, no output for `input-after` (default 10s), and the last non-blank line matches an input pattern |
| `exited` | the program is gone | the pane's process ended |

Foreground process: `tcgetpgrp(pty)` (POSIX), then the process name from
`/proc/<pid>/comm` on Linux, `ps -o comm=` on macOS. Windows' ConPTY has
no equivalent, so there `working`/`idle` come from output activity alone
and are documented as such.

Input patterns are configurable, defaulting to the common ones:

```toml
[agent]
input-patterns = ['\[[yY]/[nN]\]', '\(y/n\)', '(?i)press (enter|any key)',
                  '(?i)continue\?', '(?i)password.*:', '\?\s*$']
settle = "750ms"
input-after = "10s"
```

Heuristics are honest about being heuristics: `slat status --json`
reports `"status_reason": "fg=go, quiet 2.1s"` so an agent (or a human
debugging one) can see why.

## wait

`wait` is what replaces polling `capture` in a loop:

- `--for idle` — the pane's program finished and the shell is back.
- `--for input` — it stopped to ask a human something.
- `--for exit` — the pane's program exited (the pane closes).
- `--for text=REGEX` — the regex matched new output since `wait` started.
- `--timeout` — default 60s; exit code 2, and the final status is still printed.

`--for idle` implies "or input": an agent waiting for a build to finish
should wake up when the build asks a question instead of hanging until
the timeout. The JSON says which one happened.

## Notifications

For humans watching agents, the daemon runs a configured command when a
pane changes to a state worth interrupting for:

```toml
[agent]
on-input = "notify-send 'slat: pane %p needs input' '%c'"
on-idle  = ""
```

`%p` pane id, `%t` tab name, `%c` last command line, `%s` status. The
hook runs detached; its output is ignored; failures are logged to the
daemon log, never to the screen.

## Transport

The daemon already listens on a per-user socket (0600) and speaks
length-prefixed frames. Control adds one frame type:

- client → daemon `TypeControl` (0x04): a JSON request.
- daemon → client: a JSON response frame, then the connection closes.

A control connection never attaches to the session's output, never kicks
the interactive client, and never renders. `slat` decides which kind of
connection to make from its arguments, so agents and humans share one
binary and one socket.

### Security

A control connection can run commands in a pane, so it is exactly as
powerful as the user's shell. Two properties keep that honest:

- the socket is 0600, owned by the user, and there is no network listener;
- slat never sends terminal contents anywhere. `capture` prints to the
  caller's stdout, nothing else.

An agent that can talk to slat could already run commands directly. slat
does not widen the blast radius; it makes what the agent does visible in
panes a human can watch.

## What this is not

- **Not MCP.** Agents shell out; a CLI works with every one of them and
  needs no per-vendor server. An MCP wrapper over this CLI is ~100 lines
  and can live outside slat if someone wants it.
- **Not an AI feature.** slat ships no model client, no API keys, no
  network code, and no prompt handling. It is a multiplexer that is
  pleasant to drive programmatically; the intelligence stays in the agent.

## Decisions

1. `slat run` returns as soon as the command is sent. Agents compose it with
   an explicit `slat wait`, which is clearer than a hidden block and lets one
   agent start work in several panes before waiting on any of them.
2. Panes are addressed by id or `active`. Ids are unique for the life of a
   session and are never reused, so a `tab:pane` form would only add a second
   spelling for the same thing — and a vanished id is reported as exit code 3
   rather than "no such pane".
3. `capture` reads the visible screen; `--history` adds the scrollback and
   `--lines N` limits it. Blank rows below the last output are left out, so
   `--lines 3` returns three lines of text.
