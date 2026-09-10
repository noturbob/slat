<div align="center">

```text
     _____ __      ___  _______
    / ___// /     /   |/_  __/
    \__ \/ /     / /| | / /
   ___/ / /___  / ___ |/ /
  /____/_____/ /_/  |_/_/
```

### A modern terminal multiplexer with tiling panes, tabs, and workspaces — built in Go.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-blue?style=for-the-badge)](#requirements)

[Features](#features) • [Quick Start](#quick-start) • [Keybindings](#keybindings) • [Configuration](#configuration) • [Architecture](#architecture)

</div>

---

## Features

* **Tiling pane management** — split vertically / horizontally, resize, swap, equalize, zoom
* **Directional navigation** — move between panes with hjkl (vim-style) or arrow-style keys
* **Tabs** — create, rename, close, jump to tabs by number (1–9)
* **Workspaces** — organize sessions into named workspaces, switch freely
* **Interactive rename** — rename tabs and workspaces inline with a prompt bar
* **Prefix-key system** — one leader key, then a single keystroke for every action
* **Fully configurable** — TOML config for prefix key, shell, status bar, and keybinds
* **Persistent sessions** — a background daemon owns your shells; the client is just the window onto them, so closing your terminal doesn't kill your work
* **Detach / reattach** — disconnect without ending the session, then `slat` back in later exactly where you left off
* **Status bar & help overlay** — workspace, tabs, mode indicator, pane info, and a `?` keybind reference
* **Lightweight** — minimal dependencies, fast startup

## Quick Start

```bash
# Build
make build

# Run — auto-starts the background daemon on first launch
./bin/slat

# Or install to $GOPATH/bin (or ~/go/bin, or /usr/local/bin)
make install
slat
```

The first time you run `slat`, it spawns a small background daemon (over a per-user Unix socket) that owns your workspaces, tabs, and shells. Running `slat` again from any terminal attaches a fresh client to that same session — close the terminal, and the daemon keeps everything running for you to reattach to later.

## Keybindings

All commands use a **prefix key** (default: **`Ctrl-S`**). Press the prefix, then the action key.

> **Tip:** Press `Ctrl-S` then `?` inside slat to see the keybind reference at any time.

### Panes

| Key | Action                                    |
| --- | ------------------------------------------ |
| `v` | Split pane vertically (left / right)      |
| `h` | Split pane horizontally (top / bottom)    |
| `o` | Cycle focus to next pane                  |
| `O` | Cycle focus to previous pane               |
| `k` | Focus pane **above**                      |
| `j` | Focus pane **below**                      |
| `H` | Focus pane to the **left**                |
| `L` | Focus pane to the **right**                |
| `s` | Swap active pane with the next pane       |
| `+` | Grow active pane (increase split ratio)   |
| `-` | Shrink active pane (decrease split ratio) |
| `=` | Equalize all pane sizes                   |
| `z` | Toggle zoom — fullscreen the active pane  |
| `x` | Close active pane                         |

### Tabs

| Key     | Action                                |
| ------- | -------------------------------------- |
| `c`     | Create a new tab                      |
| `n`     | Switch to next tab                    |
| `p`     | Switch to previous tab                |
| `1`–`9` | Jump directly to tab #                |
| `,`     | Rename the current tab (opens prompt) |
| `X`     | Close the entire tab (all its panes)  |

### Workspaces

| Key | Action                                      |
| --- | -------------------------------------------- |
| `W` | Create a new workspace                      |
| `w` | Switch to next workspace                    |
| `P` | Switch to previous workspace                |
| `$` | Rename the current workspace (opens prompt) |

### Session

| Key          | Action                                 |
| ------------ | --------------------------------------- |
| `?`          | Show / dismiss the help overlay        |
| `d`          | Detach — disconnect, leave session running |
| `q`          | Quit slat and end the session          |
| *prefix × 2* | Send the prefix key to the shell       |

`Ctrl-C` and every other key not listed above are passed straight through to whatever's running in the active pane — slat only intercepts input right after the prefix key, so you can still interrupt a running program the normal way. To close things you use `x` (pane) or `q` (session), not `Ctrl-C`.

### Detach vs. Quit

* **`d` — Detach:** disconnects the current client while leaving the session running in the background daemon. Run `slat` again later to reattach.
* **`q` — Quit:** terminates the daemon's session and every shell in it.
* **Shell exits / last pane dies:** the session ends on its own, same as quitting.

## Configuration

Create:

```text
~/.config/slat/config.toml
```

Example:

```toml
prefix = "C-s"
shell = "/bin/bash"
status_bar = true

[keybinds]

# Panes
split-vertical    = "v"
split-horizontal  = "h"
next-pane         = "o"
prev-pane         = "O"
select-pane-up    = "k"
select-pane-down  = "j"
select-pane-left  = "H"
select-pane-right = "L"
swap-pane         = "s"
resize-grow       = "+"
resize-shrink     = "-"
equalize          = "="
close-pane        = "x"
zoom              = "z"

# Tabs
new-tab           = "c"
next-tab          = "n"
prev-tab          = "p"
rename-tab        = ","
close-tab         = "X"

# Workspaces
new-workspace     = "W"
next-workspace    = "w"
prev-workspace    = "P"
rename-workspace  = "$"

# Session
quit              = "q"
detach            = "d"
```

> Keys `1`–`9` are always hard-bound to "jump to tab N" and cannot be remapped. An empty or partial `[keybinds]` table is fine — any keys you don't set fall back to their defaults.

## Architecture

Slat is a proper client/daemon multiplexer: a single background daemon owns the session (workspaces, tabs, panes, and every shell's PTY), and lightweight clients attach to it over a per-user Unix socket to drive it interactively.

```text
                    ┌─────────────────────┐
                    │   Terminal / Client   │
                    │                       │
                    │  raw-mode stdin/stdout │
                    └──────────┬────────────┘
                               │ Unix socket
                               │ (length-prefixed frames: hello, input, resize)
                               ▼
                    ┌───────────────────────┐
                    │        Daemon         │
                    │                       │
                    │  session lifecycle    │
                    │  client attach/detach │
                    └──────────┬────────────┘
                               │
                    FeedInput([]byte) / HandleResize(...)
                               │
                    ┌──────────▼────────────┐
                    │         App           │
                    │                       │
                    │  Workspaces           │
                    │  Tabs                 │
                    │  Panes (PTY-backed)   │
                    │  Layout engine        │
                    │  Input dispatch       │
                    │  ANSI rendering       │
                    └───────────────────────┘
```

Running `slat` re-execs itself once as a detached background daemon (`slat __daemon`, hidden — you never invoke it directly) if one isn't already running for your user, then connects to it as a client. Only one client is attached at a time; attaching while another client is connected disconnects the previous one, the same way `tmux attach` does.

### Internal structure

```text
cmd/slat/          — Entry point: daemon bootstrap + client connect
internal/
  daemon/          — Unix socket server, client attach/detach, session lifecycle
  client/          — Terminal raw mode, stdin/stdout <-> daemon framing
  proto/           — Length-prefixed frame protocol between client and daemon
  app/             — Session engine: rendering, input dispatch, lifecycle
  config/          — TOML configuration loading with defaults
  input/           — Prefix-key handler and keybind action mapping
  layout/          — Binary-tree tiling layout engine
  pane/            — PTY-backed terminal panes, per-pane cursor tracking
  session/         — Workspace / tab / pane manager
  ui/              — ANSI rendering, status bar, help overlay, banner
```

### Rendering model

Each pane streams its shell's raw output directly onto the shared terminal — there's no full per-pane screen buffer to redraw from. To keep that correct, slat tracks a lightweight cursor position per pane (`internal/pane/cursor.go`) so it always knows exactly where a pane's shell believes its own cursor is, and explicitly repositions the real terminal cursor there before every write. It also confines each pane's scroll region and, for panes that don't span the full terminal width, its left/right margins (DECSLRM, on terminals that support it) — so a pane's content wraps and scrolls within its own borders instead of bleeding into its neighbors.

### App lifecycle

The application layer doesn't own a terminal directly — it's driven entirely by its host (the daemon):

```go
cfg, err := config.Load()
app, err := app.New(cfg)

app.SetOutput(clientWriter)
app.Start(cols, rows)

// Host feeds input and resize events as they arrive:
app.FeedInput(data)
app.HandleResize(cols, rows)

// Host watches lifecycle events:
select {
case <-app.Done():
    // Session has ended entirely.
case <-app.DetachRequested():
    // Disconnect the current client; session stays alive.
}

// On shutdown, the host terminates every shell:
app.Shutdown()
```

## Requirements

* Go 1.22+
* Linux or macOS
* Windows not yet supported

## License

MIT
