<div align="center">

<img src="assets/logo.svg" width="96" height="96" alt="slat logo">

# slat

**Split your terminal. Keep it running when you leave.**

A terminal multiplexer with tiling panes, tabs and workspaces, written in Go.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS-blue?style=flat-square)](#requirements)
[![Ko-fi](https://img.shields.io/badge/Ko--fi-support%20slat-ff5e5b?style=flat-square&logo=ko-fi&logoColor=white)](https://ko-fi.com/bobbyanthene)

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

| Platform | How |
| --- | --- |
| Debian / Ubuntu | [signed apt repository](#debian--ubuntu) |
| Arch Linux | [package from the latest release](#arch-linux) |
| Fedora / RHEL | [one `dnf` command](#fedora--rhel) |
| macOS, anything with Go | [`go install`](#with-go) |
| Try it without installing | [Docker](#try-it-in-docker) |

Every package comes for both `amd64` and `arm64`.

### Debian / Ubuntu

Add the repository once; after that, `apt upgrade` keeps slat up to date.

```bash
# 1. Trust the repository's signing key
curl -fsSL https://noturbob.github.io/slat/apt/slat.gpg \
  | sudo tee /usr/share/keyrings/slat.gpg > /dev/null

# 2. Add the repository
repo=https://noturbob.github.io/slat/apt
echo "deb [signed-by=/usr/share/keyrings/slat.gpg] $repo stable main" \
  | sudo tee /etc/apt/sources.list.d/slat.list

# 3. Install
sudo apt update
sudo apt install slat
```

<details>
<summary>Key fingerprint, or install a single <code>.deb</code> instead</summary>

<br>

The repository key's fingerprint is:

```text
9040 A503 5BDC A04A 55C3  719A D1A8 1309 D2F2 0668
```

To install one release without adding the repository:

```bash
curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.deb
sudo apt install ./slat_linux_amd64.deb
```

</details>

### Arch Linux

```bash
curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.pkg.tar.zst
sudo pacman -U slat_linux_amd64.pkg.tar.zst
```

An AUR package (`yay -S slat`) is on the way.

### Windows

Download `slat_windows_amd64.zip` from the
[latest release](https://github.com/noturbob/slat/releases/latest), unzip it and
put `slat.exe` on your `PATH`. Windows 10 1809 or later is required (that's when
ConPTY arrived). Panes run `cmd.exe` by default; for something else:

```toml
# %USERPROFILE%\.config\slat\config.toml
shell = "powershell.exe"
```

Windows Terminal is recommended — the old conhost window can't render
everything slat draws.

### Fedora / RHEL

```bash
sudo dnf install \
  https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.rpm
```

### With Go

Works on Linux and macOS:

```bash
go install github.com/noturbob/slat/cmd/slat@latest
```

<details>
<summary>Other ways: prebuilt binaries, or build from source</summary>

<br>

Prebuilt binaries for Linux and macOS are on the
[releases page](https://github.com/noturbob/slat/releases).

To build from a checkout:

```bash
make build      # ./bin/slat
make install    # into $(go env GOPATH)/bin
```

</details>

### Try it in Docker

```bash
docker run -it --rm ghcr.io/noturbob/slat
```

The shells run inside the container, not on your machine, and the container
stops when you detach, so this is for trying slat out. Install it natively for
real use. (The image is published from the next release onwards.)

### First run

```bash
slat
```

The first run starts the background daemon; later runs attach to it.
Press <kbd>Ctrl</kbd>+<kbd>S</kbd> then <kbd>?</kbd> inside slat to see every key.

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

## Scripting and agents

Every command below talks to a running session over its socket, so scripts — and
AI coding agents — can drive slat without a terminal attached. Nothing is sent
anywhere: slat has no network code.

```console
$ slat ls
*1   idle     1:shell    bash    ~/src/slat
 2   working  1:build    go      ~/src/slat

$ slat pane new --cmd 'go test ./...'   # new pane, your focus stays put
$ slat wait 3 --for idle --timeout 5m   # block until it finishes
$ slat capture 3 --lines 20             # read what it printed
```

| Command | What it does |
| --- | --- |
| `slat ls` | every pane, with what each one is doing |
| `slat status [PANE]` | one pane's status |
| `slat pane new [--split v\|h] [--cwd D] [--cmd C] [--target P] [--focus]` | split a pane |
| `slat pane close PANE` | close a pane |
| `slat send PANE TEXT [--enter] [--key KEY]` | type into a pane |
| `slat run PANE COMMAND...` | send a command and press Enter |
| `slat capture PANE [--lines N] [--history]` | read a pane's text |
| `slat wait PANE --for idle\|input\|exit\|text=REGEX [--timeout 60s]` | block until something happens |

`PANE` is an id from `slat ls`, or `active` (the default). `--json` on any
command prints one object with a `schema` field. Exit codes: **0** ok,
**1** error, **2** timed out, **3** that pane is gone.

A pane's status is one of four, which is what makes `wait` useful:

- **idle** — the pane's own shell has the terminal: the command finished.
- **working** — a program is running.
- **input** — that program has gone quiet on a question (`[y/N]`, a password
  prompt), so an agent waiting for a build wakes up instead of hanging.
- **exited** — the pane's program is gone.

When a pane starts waiting for input, its tab is marked `2:shell ?` in the
status bar, and slat can run a command so you don't have to watch:

```toml
[agent]
on_input = "notify-send \"slat: pane %p needs input\" %c"
```

`%p` is the pane id, `%t` its tab, `%s` the status and `%c` the pane's last
line. Values are shell-quoted for you, so a prompt containing a quote stays
text. The hook runs detached; its output goes to the daemon log, never to your
screen.

Tune the detection under `[agent]` in the config; see
[docs/design/agent-cli.md](docs/design/agent-cli.md) for the full design.

## Configuration

slat reads `~/.config/slat/config.toml` (or `$XDG_CONFIG_HOME/slat/config.toml`).
Every setting is optional:

```toml
prefix     = "C-a"        # Ctrl + a letter, or C-\ C-] C-^ C-_
shell      = "/bin/zsh"   # default: $SHELL
status_bar = true
scrollback = 5000         # lines kept per pane; 0 turns it off

[agent]                   # how `slat status` reads a pane (see above)
settle      = "750ms"     # quiet for this long after output = idle
input_after = "10s"       # quiet for this long on a prompt = waiting for input

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
- On Windows, ConPTY has no foreground process group, so a pane's status
  comes from output timing alone: `slat status` says idle or working, never
  which program is running, and `slat ls` shows the directory a pane started
  in rather than where its shell has since moved.

## Requirements

- Go 1.23+ to build
- Linux, macOS, or Windows 10 1809 and later

## Support

slat is free and always will be. If it saves you time, you can
[buy me a coffee on Ko-fi](https://ko-fi.com/bobbyanthene).

## License

MIT
