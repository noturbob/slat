# Slat

A modern, easy-to-use terminal multiplexer written in Go with tiling panes and workspaces.

## Features

- **Tiling window management** — split panes vertically and horizontally
- **Tabs** — multiple shell tabs per workspace
- **Workspaces** — organize your work into separate contexts
- **Simple keybindings** — prefix-based, fully customizable via TOML config
- **Lightweight** — minimal dependencies, fast startup
- **Cross-platform** — works on Linux and macOS

## Quick Start

```bash
# Build
make build

# Run
make run

# Or install to your PATH
make install
slat
```

## Default Keybindings

All commands are triggered by pressing the **prefix key** first (default: `Ctrl-S`), then the action key:

| Key | Action |
|-----|--------|
| `v` | Split pane vertically |
| `h` | Split pane horizontally |
| `o` | Next pane |
| `O` | Previous pane |
| `x` | Close current pane |
| `c` | New tab |
| `n` | Next tab |
| `p` | Previous tab |
| `W` | New workspace |
| `w` | Next workspace |
| `z` | Zoom/fullscreen pane |
| `q` | Quit |
| `?` | Show help |

## Configuration

Create `~/.config/slat/config.toml`:

```toml
prefix = "C-s"
shell = "/bin/bash"
status_bar = true

[keybinds]
split-vertical   = "v"
split-horizontal = "h"
next-pane        = "o"
prev-pane        = "O"
close-pane       = "x"
new-tab          = "c"
next-tab         = "n"
prev-tab         = "p"
new-workspace    = "W"
next-workspace   = "w"
zoom             = "z"
quit             = "q"
detach           = "d"
```

## Architecture

```
cmd/slat/          — Entry point
internal/
  app/             — Main application loop (raw mode, signals, render)
  config/          — TOML configuration loading
  input/           — Input handling and keybindings
  layout/          — Binary tree tiling layout engine
  pane/            — PTY-backed terminal panes
  session/         — Workspace/tab/pane management
  ui/              — Terminal rendering and status bar
```

## Requirements

- Go 1.22+
- Linux or macOS (Windows not yet supported)

## License
MIT