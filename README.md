# Slat

A modern terminal multiplexer with tiling panes, tabs, and workspaces — built in Go.

```text
     _____ __      ___  _______
    / ___// /     /   |/_  __/
    \__ \/ /     / /| | / /
   ___/ / /___  / ___ |/ /
  /____/_____/ /_/  |_/_/
```

## Features

* **Tiling pane management** — split vertically / horizontally, resize, swap, equalize, zoom
* **Directional navigation** — move between panes with hjkl (vim-style) or arrow-style keys
* **Tabs** — create, rename, close, jump to tabs by number (1-9)
* **Workspaces** — organize sessions into named workspaces, switch freely
* **Interactive rename** — rename tabs and workspaces inline with a prompt bar
* **Prefix-key system** — one leader key, then a single keystroke for every action
* **Fully configurable** — TOML config for prefix key, shell, status bar, and keybinds
* **Beautiful status bar** — workspace, tabs, mode indicator, pane information
* **Help overlay** — press `?` for a categorized keybind reference
* **Zoom mode** — fullscreen the active pane and toggle back to the previous layout
* **Session engine** — application state is separated from terminal I/O, allowing a host/daemon to manage sessions independently from clients
* **Detach support** — clients can request a detach without terminating the underlying session
* **Lightweight** — minimal dependencies and fast startup

> **Current status:** Slat's application layer is now structured as a host-driven session engine. The `App` no longer owns stdin/stdout or terminal signal handling directly. A daemon/client layer can attach to it through its input, output, resize, detach, and lifecycle APIs.

## Quick Start

```bash
# Build
make build

# Run
./bin/slat

# Or install to ~/go/bin
make install
slat
```

## Keybindings

All commands use a **prefix key** (default: **`Ctrl-S`**). Press the prefix, then the action key.

> **Tip:** Press `Ctrl-S` then `?` inside slat to see the keybind reference at any time.

### Panes

| Key | Action                                    |
| --- | ----------------------------------------- |
| `v` | Split pane vertically (left / right)      |
| `h` | Split pane horizontally (top / bottom)    |
| `o` | Cycle focus to next pane                  |
| `O` | Cycle focus to previous pane              |
| `k` | Focus pane **above**                      |
| `j` | Focus pane **below**                      |
| `H` | Focus pane to the **left**                |
| `L` | Focus pane to the **right**               |
| `s` | Swap active pane with the next pane       |
| `+` | Grow active pane (increase split ratio)   |
| `-` | Shrink active pane (decrease split ratio) |
| `=` | Equalize all pane sizes                   |
| `z` | Toggle zoom — fullscreen the active pane  |
| `x` | Close active pane                         |

### Tabs

| Key     | Action                                |
| ------- | ------------------------------------- |
| `c`     | Create a new tab                      |
| `n`     | Switch to next tab                    |
| `p`     | Switch to previous tab                |
| `1`–`9` | Jump directly to tab #                |
| `,`     | Rename the current tab (opens prompt) |
| `X`     | Close the entire tab (all its panes)  |

### Workspaces

| Key | Action                                      |
| --- | ------------------------------------------- |
| `W` | Create a new workspace                      |
| `w` | Switch to next workspace                    |
| `P` | Switch to previous workspace                |
| `$` | Rename the current workspace (opens prompt) |

### Session

| Key          | Action                                 |
| ------------ | -------------------------------------- |
| `?`          | Show / dismiss the help overlay        |
| `d`          | Request detach from the current client |
| `q`          | Quit slat and end the session          |
| *prefix × 2* | Send the prefix key to the shell       |

### Detach vs. Quit

`d` and `q` have different meanings:

* **`d` — Detach:** requests that the current client disconnect while leaving the session running.
* **`q` — Quit:** terminates the Slat session.
* **Shell exits / last pane dies:** the session reports that it is finished.

The application layer exposes separate lifecycle channels for these events:

```text
Done()
    └── Session has ended

DetachRequested()
    └── Current client should disconnect
        but the session remains alive
```

The host/daemon is responsible for deciding how clients attach, detach, reconnect, and where session output is delivered.

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

> Keys `1`–`9` are always hard-bound to "jump to tab N" and cannot be remapped.

## Architecture

Slat's application layer is separated from terminal/client I/O. The `App` owns the session state and can be driven by a host such as a terminal daemon.

```text
                    ┌─────────────────────┐
                    │     Client / TTY     │
                    │                     │
                    │ keyboard            │
                    │ terminal size       │
                    │ output              │
                    └──────────┬──────────┘
                               │
                         host / daemon
                               │
              ┌────────────────┴────────────────┐
              │                                 │
              ▼                                 ▼
       FeedInput([]byte)                 HandleResize(...)
              │                                 │
              └──────────────┬──────────────────┘
                             ▼
                    ┌─────────────────┐
                    │      App        │
                    │                 │
                    │ Workspaces      │
                    │ Tabs            │
                    │ Panes           │
                    │ Layout          │
                    │ Input handling  │
                    │ Rendering       │
                    └────────┬────────┘
                             │
                    SetOutput(io.Writer)
                             │
                             ▼
                         Client
```

### Internal structure

```text
cmd/slat/          — Entry point / host
internal/
  app/             — Session engine, rendering, input dispatch, lifecycle
  config/          — TOML configuration loading with defaults
  input/           — Prefix-key handler and keybind action mapping
  layout/          — Binary-tree tiling layout engine
  pane/            — PTY-backed terminal panes
  session/         — Workspace / tab / pane manager
  ui/              — ANSI rendering, status bar, help overlay
```

### App lifecycle

The application is driven by its host rather than directly reading from a terminal.

```go
cfg, err := config.Load()
if err != nil {
    return err
}

app, err := app.New(cfg)
if err != nil {
    return err
}

app.SetOutput(clientWriter)

if err := app.Start(cols, rows); err != nil {
    return err
}
```

Input is supplied by the host:

```go
app.FeedInput(data)
```

Terminal resize events are supplied by the host:

```go
app.HandleResize(cols, rows)
```

The host can monitor session lifecycle events:

```go
select {
case <-app.Done():
    // Session ended.

case <-app.DetachRequested():
    // Disconnect the current client.
    // Keep the session alive.
}
```

The host can explicitly terminate all shell processes with:

```go
app.Shutdown()
```

> **Note:** The application layer supports the lifecycle required for persistent sessions, but persistence across client connections depends on the host/daemon implementation. Unix-socket transport and automatic reattachment should only be considered available once implemented by the host.

## Bug Fixes

This version fixes several correctness, stability, and lifecycle issues discovered during development.

### Configuration

* **Panic on custom config** — if `config.toml` omitted `[keybinds]`, the decoded keybind map could be nil and writes to it could panic during startup. Configuration handling now safely handles missing keybind configuration.

### Input and shutdown

* **App could hang on exit** — direct `os.Stdin.Read` could block indefinitely after the application had already decided to quit. Input handling was moved away from the application layer so the host controls input delivery and session shutdown.
* **Input fast-path bug** — bulk-forwarding an entire input buffer could swallow a prefix byte that arrived in the middle of the buffer. Input forwarding now stops before the next prefix byte so prefix commands remain reliable during fast typing and paste operations.
* **Detach support** — detach is now a distinct lifecycle event from quitting. Detaching does not close the session's `Done()` channel or terminate the shell processes.

### Process and PTY lifecycle

* **Orphaned shell processes** — quitting previously did not reliably terminate spawned shell processes and PTYs. `Manager.Shutdown()` is now exposed through `App.Shutdown()` so the host can explicitly terminate the entire session.
* **PTY file descriptor leak** — PTY master file descriptors could remain open when shells exited naturally instead of being closed through an explicit pane-close operation.

### Zoom

* **Fake zoom** — `z` previously resized the active pane but did not track its zoom state, making it impossible to restore the previous layout. Zoom is now a real toggle with explicit state tracking.
* **Zoomed pane resize** — terminal resizing while zoomed now keeps the zoomed pane fullscreen instead of applying the normal layout tree.
* **Dead zoomed pane** — if a zoomed shell exits, the zoom state is cleared before the layout is restored.

### Concurrency

* **Data race on help state** — help visibility was accessed from multiple execution paths without synchronization. It now uses `atomic.Bool`.
* **Terminal/output ownership** — terminal output is synchronized through a single buffered writer and mutex, allowing the host to provide the output destination instead of `App` directly owning `os.Stdout`.

### Layout and rendering

* **Status bar disabled but still consumed a row** — `status_bar = false` previously still reserved one terminal row. Pane dimensions now correctly use the full terminal height when the status bar is disabled.
* **Status bar overflow** — a large number of tabs could cause status-bar content to overlap or wrap. Status rendering now budgets available width and truncates content where necessary.
* **Help overlay lost on resize** — resizing the terminal previously cleared the help overlay without redrawing it.
* **Rename prompt lost on resize** — the active rename prompt is now redrawn after terminal resizing.
* **Banner line count** — banner positioning previously relied on a hardcoded line count. The actual banner contents are now used to determine its dimensions.

## Current Architecture Status

Slat is being developed toward a persistent client/server architecture.

The application/session layer already exposes the core lifecycle required by a daemon:

* `Start(cols, rows)`
* `SetOutput(io.Writer)`
* `FeedInput([]byte)`
* `HandleResize(cols, rows)`
* `Done()`
* `DetachRequested()`
* `Shutdown()`

The remaining host-side responsibilities for full persistence include:

* Unix socket server
* Client attach protocol
* Client detach/reattach
* Session identification
* Session discovery/listing
* Routing input/output between clients and sessions
* Surviving client terminal crashes
* Daemon lifecycle management

Until those pieces are implemented, Slat should **not** be considered a fully persistent tmux-style daemon.

## Requirements

* Go 1.22+
* Linux or macOS
* Windows not yet supported

## License

MIT
