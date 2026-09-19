<div align="center">

<img src="assets/logo.svg" width="96" height="96" alt="slat logo">

# slat

**Split your terminal. Keep it running when you leave.**

A terminal multiplexer with tiling panes, tabs and workspaces, written in Go.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-blue?style=flat-square)](#requirements)

[Website](https://noturbob.github.io/slat/) · [Install](#install) · [Keys](#keys) · [Configuration](#configuration) · [How it works](#how-it-works)

</div>

<p align="center">
  <img src="assets/demo.gif" alt="slat splitting a terminal into panes, running htop and go test side by side, scrolling back and searching the output, then detaching and reattaching with everything still running" width="100%">
</p>

---

slat tiles one terminal into panes, tabs and workspaces. Your shells live in a
small background process, so closing the window — or losing the SSH
connection — doesn't end anything. Run `slat` again and it's all there,
output included.

- **Panes** — split left/right or top/bottom, move between them by direction, swap, resize, zoom
- **Tabs and workspaces** — group tabs into named workspaces, rename them in place
- **Detach and reattach** — `d` disconnects; running `slat` from any terminal reattaches
- **Scroll mode** — page back through each pane's output and search it, vim-style
- **Every pane keeps its own screen** — `clear`, vim or htop in one pane never touch another, and nothing is lost when you split, close or switch
- **Fast** — only changed cells are sent to your terminal; half a million lines of output render in about a third of a second
- **New panes open where you are** — a split starts in the directory of the pane you split from (Linux)
- **One checked config file** — mistakes are reported when you run `slat`, not ignored

## Install

**Arch Linux:**

```bash
curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.pkg.tar.zst
sudo pacman -U slat_linux_amd64.pkg.tar.zst
```

An AUR package (`yay -S slat`) is on the way.

**Debian / Ubuntu** (signed apt repository; `apt upgrade` keeps it current):

```bash
curl -fsSL https://noturbob.github.io/slat/apt/slat.gpg | sudo tee /usr/share/keyrings/slat.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/slat.gpg] https://noturbob.github.io/slat/apt stable main" | sudo tee /etc/apt/sources.list.d/slat.list
sudo apt update && sudo apt install slat
```

The repository key's fingerprint is `9040 A503 5BDC A04A 55C3  719A D1A8 1309 D2F2 0668`.
Or install a single `.deb` without the repository:
`curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.deb && sudo apt install ./slat_linux_amd64.deb`

**Fedora / RHEL:**

```bash
sudo dnf install https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.rpm
```

**With Go** (any platform):

```bash
go install github.com/noturbob/slat/cmd/slat@latest
```

**macOS / other Linux:** grab a binary from the [releases page](https://github.com/noturbob/slat/releases).
Every package above also comes in an `arm64` build.

**From a checkout:**

```bash
make build      # ./bin/slat
make install    # into $(go env GOPATH)/bin
```

Then run `slat`. The first run starts the background daemon; later runs attach
to it.

## Keys

Press the **prefix** (default <kbd>Ctrl</kbd>+<kbd>S</kbd>), then one key.
Everything else goes straight to your shell — including <kbd>Ctrl</kbd>+<kbd>C</kbd>.
Press the prefix twice to send it to the shell. Prefix then <kbd>?</kbd> shows
every key inside slat.

### Panes

| Key | Action |
| --- | --- |
| `v` / `h` | Split left / right, top / bottom |
| `o` / `O` | Next / previous pane |
| arrows, or `k` `j` `H` `L` | Pane above / below / left / right |
| `s` | Swap with the next pane |
| `+` / `-` | Grow / shrink the pane |
| `=` | Equalize all sizes |
| `z` | Zoom the pane to full size (toggle) |
| `x` | Close the pane |
| `[` or PageUp | Scroll mode (see below) |

### Tabs

| Key | Action |
| --- | --- |
| `c` | New tab |
| `n` / `p` | Next / previous tab |
| `1`–`9` | Go to tab N |
| `,` | Rename the tab |
| `X` | Close the tab and all its panes |

### Workspaces

| Key | Action |
| --- | --- |
| `W` | New workspace |
| `w` / `P` | Next / previous workspace |
| `$` | Rename the workspace |

### Session

| Key | Action |
| --- | --- |
| `d` | Detach — the session keeps running |
| `q` | Quit — ends every shell |
| `?` | Show all keys |

### Scroll mode

Prefix then `[` (or PageUp) shows the active pane's earlier output. The view
stays put while new output arrives. Your mouse wheel scrolls too, in terminals
that send arrow keys for it.

| Key | Action |
| --- | --- |
| `k` `j`, arrows | Up / down a line |
| `Ctrl-U` `Ctrl-D`, `u` `d` | Up / down half a page |
| `Ctrl-B` `Ctrl-F`, PageUp PageDown, `b` `f` space | Up / down a page |
| `g` `G`, Home End | Oldest output / live screen |
| `/` `?` | Search up / down (a query with no capitals ignores case) |
| `n` `N` | Next match in the same / opposite direction |
| `q`, Esc | Leave scroll mode |

Each pane keeps 2000 lines by default (`scrollback` in the config). Full-screen
programs like vim and less use their own screen and add nothing to it, and
`clear` wipes it, as in most terminals.

In a rename prompt: <kbd>Enter</kbd> saves, <kbd>Esc</kbd> or <kbd>Ctrl</kbd>+<kbd>C</kbd>
cancels, <kbd>Ctrl</kbd>+<kbd>U</kbd> clears.

The session ends by itself when its last shell exits. Closing the last tab of a
workspace removes just that workspace.

## Configuration

slat reads `~/.config/slat/config.toml` (or `$XDG_CONFIG_HOME/slat/config.toml`).
Every setting is optional:

```toml
prefix     = "C-a"        # Ctrl + a letter, or C-\ C-] C-^ C-_
shell      = "/bin/zsh"   # default: $SHELL
status_bar = true
scrollback = 5000         # lines kept per pane; 0 turns it off

[keybinds]
split-vertical   = "|"
split-horizontal = "-"
resize-shrink    = "_"    # "-" was given away above
zoom             = ""     # "" unbinds a command
```

- Keybinds you don't set keep their defaults.
- Giving a default key to another command takes it away from the default
  command, so bindings never silently collide.
- An unknown setting, an unknown command name, a key that isn't a single
  character, or two commands on one key is an error, shown when you run `slat`.

[`config.example.toml`](config.example.toml) lists every command with its default.

## How it works

```text
 terminal ── slat (client) ──unix socket──▶ slat daemon
   raw keys ─────────────────────────────▶   ├─ workspaces / tabs / layout tree
   ◀── only the changed cells ─────────────  ├─ per-pane terminal emulator
                                             └─ shells on PTYs
```

- **Daemon.** The first `slat` re-executes itself as a background daemon that owns
  every shell. Its socket lives in `$XDG_RUNTIME_DIR` (or the temp dir) and is
  only accessible to you. Attaching from a second terminal takes over from the
  first, like `tmux attach -d`. The daemon's own errors go to the `.log` file
  next to its socket.
- **Emulation.** Each pane's output is fed into slat's own terminal emulator
  (`internal/vt`), which keeps that pane's screen. It handles the xterm features
  shells and full-screen programs use: colors (16, 256 and 24-bit), wide
  characters and emoji, scroll regions, the alternate screen, line drawing and
  cursor queries.
- **Rendering.** On every change slat composes a frame from the panes, borders,
  status bar and overlays, compares it with what your terminal already shows,
  and sends only the difference. Since frames are built from state rather than
  patched step by step, a split, close, resize or reattach can't leave stale
  or missing text behind.
- **Input modes.** The active pane's cursor-key mode and bracketed paste are
  mirrored onto your terminal, so arrow keys in vim and multi-line pastes into
  your shell behave as they do outside slat.

### Source layout

```text
cmd/slat/        entry point: starts or attaches to the daemon
internal/
  daemon/        socket server, attach / detach, session lifetime
  client/        raw mode, alternate screen, stdin/stdout ⇄ daemon
  proto/         length-prefixed frames from client to daemon
  app/           the session: input → actions, frame composition
  vt/            terminal emulator for each pane
  ui/            frame, diffing renderer, status bar, help, borders
  session/       workspaces, tabs, focus, removing dead panes
  layout/        binary-tree tiling
  pane/          a shell on a PTY with its emulator
  input/         prefix key and command table
  config/        TOML loading and validation
docs/            the website (GitHub Pages)
packaging/aur/   the AUR package
packaging/apt/   builds the signed apt repository
```

Releases are built by [GoReleaser](.goreleaser.yaml) when a `v*` tag is pushed:
binaries, `.deb`, `.rpm` and Arch packages for Linux and macOS on amd64 and arm64.
The release then republishes the website and the apt repository to GitHub Pages
([`pages.yml`](.github/workflows/pages.yml)), signed with the `APT_GPG_PRIVATE_KEY` secret.

`make test` runs `go vet` and the tests with the race detector. The tests in
`internal/app` drive real shells through splits, closes, overlays and
workspace changes and check what a terminal would display.

## Limitations

- Scroll mode can't select and copy text yet.
- Mouse events aren't passed to programs in panes.
- Windows isn't supported.

## Requirements

- Go 1.23+ to build
- Linux or macOS

## License

MIT
