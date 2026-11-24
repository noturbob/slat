<div align="center">

<img src="docs/banner.svg" alt="slat — split your terminal, keep it running when you leave" width="100%">

<br>

A terminal multiplexer with tiling panes, tabs and workspaces, written in Go.

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue?style=flat-square)](#requirements)
[![Ko-fi](https://img.shields.io/badge/Ko--fi-support%20slat-ff5e5b?style=flat-square&logo=ko-fi&logoColor=white)](https://ko-fi.com/bobbyanthene)

[Website](https://noturbob.github.io/slat/) · [Install](#install) · [Keys](#keys) · [Configuration](#configuration) · [How it works](#how-it-works)

</div>

<p align="center">
  <img src="docs/demo.gif" alt="slat splitting one terminal into panes, walking a pane around the layout, naming tabs and workspaces, searching and copying from the scrollback, detaching and reattaching, being driven by a script, and changing palette" width="100%">
</p>

<p align="center"><sub>A real session, captured through slat's own emulator and cut into a film — not a mock-up.</sub></p>

---

slat tiles one terminal into panes, tabs and workspaces. Your shells live in a
small background process, so closing the window — or losing the SSH
connection — doesn't end anything. Run `slat` again and it's all there,
output included.

- **Panes** — split left/right or top/bottom, move between them by direction, swap, resize, zoom
- **Tabs and workspaces** — group tabs into named workspaces, rename them in place
- **Detach and reattach** — `d` disconnects; running `slat` from any terminal reattaches
- **Survives a reboot** — the layout, each pane's directory and the output it had printed are saved and put back when slat starts again
- **Scroll and copy mode** — page back through a pane's output, search it vim-style, select characters or lines and yank them to your system clipboard (over `ssh` too, via OSC 52)
- **Mouse** — click a pane to focus it, drag a border to resize, wheel through its history; programs that want the mouse themselves get the events, translated to their pane
- **Every pane keeps its own screen** — `clear`, vim or htop in one pane never touch another, and nothing is lost when you split, close or switch
- **Fast** — only changed cells are sent to your terminal; half a million lines of output render in about a third of a second
- **New panes open where you are** — a split starts in the directory of the pane you split from (Linux)
- **Drivable from scripts and AI agents** — `slat ls`, `run`, `capture` and `wait --for idle` let anything outside the terminal work a session and know when a pane needs a human ([details](#scripting-and-agents))
- **Move panes around** — walk a pane through the layout like a tiling window manager; the swap slides instead of jumping
- **Rice it** — ten palettes or your own colours, six border styles or your own
  glyphs, a status bar you write as a format string, and animation timing and
  easing you can tune or switch off entirely
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

Or from the [AUR](https://aur.archlinux.org/packages/slat): `yay -S slat`.

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
| `a` | Add a pane (honoring the active tab's layout) |
| `o` / `O` | Next / previous pane |
| arrows, or `k` `j` `H` `L` | Pane above / below / left / right |
| `s` | Swap with the next pane |
| `m` | Move mode: `h` `j` `k` `l` walk the pane around, `q` leaves |
| `<` `>` `K` `J` | Move the pane left / right / up / down |
| `+` / `-` | Grow / shrink the pane |
| `=` | Equalize all sizes |
| `l` | Choose layout (picker menu) |
| `z` | Zoom the pane to full size per tab (toggle) |
| `t` | Toggle stacked mode (jump elements with `o`/`O`/`j`/`k` or click) |
| `!` | Break active pane into a new tab |
| `@` | Join a pane from another tab into the current tab |
| `x` | Close the pane |
| `[` or PageUp | Scroll and copy mode (see below) |

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
| `S` | Rename the session |
| `f` | Tree picker (session / workspaces / tabs / panes; `PageUp`/`PageDown` scrolls by page) |
| `F` | Switch between running sessions |
| `)` / `(` | Switch to next / previous running session |
| `g` | Global tree picker across all sessions, tabs & panes |
| `E` | Save current session as a preconfigured TOML (asks before overwriting) |
| `r` | Reload configuration (`config.toml` and themes) |
| `b` | Toggle the bottom status bar |
| `d` | Detach — the session keeps running |
| `q` | Quit — ends every shell |
| `?` | Show all keys |

Picker dialogs (`l`, `f`, `F`, `g`) filter by **word-prefix** with `fzf`-style extended search: space-separated terms are `AND`ed, `|` is `OR`, `(...)` groups expressions, `^term` matches the start, `term$` matches the end, `'term` matches any exact substring, `!term` excludes matches, `\` escapes special characters (`(`, `)`, `$`, `^`, `'`, `!`, `|`, space, `\`), and smart-case is used (case-insensitive unless uppercase is typed). Use `Ctrl+W` to delete the last word and `Ctrl+U` to clear the input.

### Scroll and copy mode

`[` (or PageUp) scrolls back through a pane's own history. The view is
anchored to the output, so it holds still while the program keeps writing.

| Key | Action |
| --- | --- |
| `k` `j` `h` `l`, arrows | Move the cursor; the view follows it |
| `Ctrl`+`u` / `Ctrl`+`d` | Half a screen up / down |
| `Ctrl`+`b` / `Ctrl`+`f`, PageUp / PageDown | A screen up / down |
| `g` / `G` | Oldest line / back to the live screen |
| `0` / `$` | Start / end of the line |
| `/` `?` | Search up / down — smart case; the cursor moves to the match |
| `n` / `N` | Next / previous match |
| `v` / `V` | Select characters / whole lines |
| `y` | Copy the selection and leave |
| `Esc` | Drop the selection, then leave |
| `q` | Leave |

Copying uses OSC 52, which asks your terminal to put the text on the system
clipboard — so it works over `ssh`, where slat runs on the far machine and
the clipboard is on yours. Some terminals refuse OSC 52 or need it enabled
(`set -g set-clipboard on` territory); for those, name a local command:

```toml
[copy]
osc52   = true          # ask the terminal; the only way that works over ssh
command = "wl-copy"     # …and/or pipe it locally: xclip -sel clip, pbcopy
```

### Session restore

A reboot used to end everything: the daemon holds your shells in memory, so
it outlives a closed window or a dropped connection, but not a restart.

slat now writes the session out — workspaces, tabs, the layout, each pane's
working directory and the output it had printed — and puts it back the next
time it starts. Panes come back in the same shape, in the same directories,
with their old output above the prompt and a dim rule marking where the
previous session ended.

What comes back is the **output, not the programs**. Nothing portable can
restore a running process, so a pane that was running `vim` comes back as a
shell in the same directory, with what `vim` had drawn left behind as text.

Quitting with <kbd>q</kbd> clears the saved session — ending a session
means ending it. Detaching, closing the window and rebooting do not. The
file lives in `$XDG_STATE_HOME/slat/session.json` (`~/.local/state` by
default), is written every 15 seconds while anything changes and again on
the way out, and keeps up to 2000 lines per pane.

```toml
restore = false     # start clean every time
```

### Layouts

Press <kbd>Ctrl</kbd>+<kbd>s</kbd> then <kbd>l</kbd> to open the fuzzy layout
picker and switch the active tab between 8 built-in layouts:

| Layout | Description |
| --- | --- |
| `master-left` | Master pane on the left, stack on the right |
| `master-right` | Master pane on the right, stack on the left |
| `master-top` | Master pane on the top, stack on the bottom |
| `master-bottom` | Master pane on the bottom, stack on the top |
| `even-horizontal` | All panes side-by-side in equal columns |
| `even-vertical` | All panes stacked top-to-bottom in equal rows (default) |
| `grid` | Every pane equal in a balanced grid |
| `stack` | Active pane full-screen with clickable header tabs |

### Preconfigured sessions

Save reusable session definitions as TOML files in `~/.config/slat/sessions/`
(or anywhere on disk — see [`slat.session_example.toml`](slat.session_example.toml)):

```toml
session_name = "sample-session"
start_directory = "~/code/10-apps/project"
# focus = 1 # 1-based tab index (or workspace index when [[workspaces]] is used) to focus when the session starts

[[tabs]]
name = "doc"
layout = "stack" # defaults to "even-vertical"
panes = [
  "nvim README.md",
]

[[tabs]]
name = "editor"
# focus = 1 # 1-based pane index to focus when the session starts
panes = [
  "nvim -S session.vim",
]
```

You can also group tabs under `[[workspaces]]`, using top-level `focus` for the 1-based workspace index and workspace-level `focus` for the 1-based tab index (see [`examples/sessions/workspaces.toml`](examples/sessions/workspaces.toml)).

Start or attach to a preconfigured session by name (resolved from
`~/.config/slat/sessions/<name>.toml`, with shell tab-completion) or by file path:

```bash
slat session slat.session_example
slat session ./slat.session_example.toml
```

Inside slat, press <kbd>Ctrl</kbd>+<kbd>s</kbd> then <kbd>E</kbd> to save the
running session to `~/.config/slat/sessions/<name>.toml`. If a file with that
name already exists, slat asks for confirmation before overwriting.

#### Shell extensions (`bash`, `zsh`, `fish`)

Source the extension for your shell to get `slat-session`, an `fzf` menu over
all `.toml` files in `~/.config/slat/sessions/` with a live file preview:

```bash
# bash (~/.bashrc)
source /path/to/slat/shell/slat.bash

# zsh (~/.zshrc)
source /path/to/slat/shell/slat.zsh
```

```fish
# fish (~/.config/fish/config.fish)
source /path/to/slat/shell/slat.fish
```

### Mouse

Click a pane to focus it, drag a border to resize, and the wheel scrolls
back through its output.
Programs that ask for the mouse — vim, htop, less — receive the events
themselves, with the coordinates translated into their pane, so they behave
as they do outside slat.

Your terminal hands its clicks to slat while this is on, so its own
click-drag selection is unavailable; hold <kbd>Shift</kbd> for it in most
terminals. To keep the terminal's mouse instead:

```toml
mouse = false
```

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
| `slat sessions [--json]` | list running directory-based sessions |
| `slat session [NAME\|PATH]` | start or attach to a preconfigured session TOML (tab-completes saved sessions in `~/.config/slat/sessions/`) |
| `slat attach [NAME\|DIR]` | attach to a session by name or directory (tab-completes running session names; falls back to current directory when omitted) |
| `slat shutdown [--all] [--session NAME\|DIR] [--json]` | shut down the active session, or all running sessions with `--all` |
| `slat status [PANE]` | one pane's status |
| `slat pane new [--split v\|h] [--cwd D] [--cmd C] [--target P] [--focus]` | split a pane |
| `slat pane close PANE` | close a pane |
| `slat send PANE TEXT... [--enter] [--key KEY]` | type into a pane (supports `--session`, `--workspace`, `--tab`, or `[SESSION:][WORKSPACE/[TAB/]]PANE`, e.g. `work:dev/active`) |
| `slat focus TARGET\|left\|down\|up\|right` | focus a pane/workspace/tab (same syntax as `slat send`) or move in a direction |
| `slat display MESSAGE [--duration 2s]` | show a temporary alert in the status bar (supports `{fg=...}` tags and `--session`) |
| `slat run PANE COMMAND...` | send a command and press Enter |
| `slat capture PANE [--lines N] [--history]` | read a pane's text |
| `slat wait PANE --for idle\|input\|exit\|text=REGEX [--timeout 60s]` | block until something happens |
| `slat theme ls [--json]` | list built-in and user themes |
| `slat theme show [NAME]` | print a theme's TOML definition to stdout |
| `slat theme export [NAME] [-o PATH] [-f]` | export a theme TOML file for customization (and show where to place it) |

`PANE` is an id from `slat ls`, or `active` (the default). `--json` on any
command prints one object with a `schema` field. Exit codes: **0** ok,
**1** error, **2** timed out, **3** that pane is gone.

A pane's status is one of four, which is what makes `wait` useful:

- **idle** — the pane's own shell has the terminal: the command finished.
- **working** — a program is running.
- **input** — something has gone quiet on a question (`[y/N]`, a password
  prompt), so an agent waiting for a build wakes up instead of hanging. Shell
  builtins count: `read -p "Overwrite? [y/N] "` is detected even though the
  shell itself is the foreground process, because the line doesn't end the way
  a prompt does.
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

slat reads `~/.config/slat/config.toml` (or `$XDG_CONFIG_HOME/slat/config.toml`), or pass `-c` / `--config PATH` (for example, `slat --config examples/configs/cyberpunk.toml`). Press <kbd>Ctrl</kbd>+<kbd>s</kbd> then <kbd>r</kbd> (`reload-config`) to reload `config.toml` and theme files in `~/.config/slat/themes/` while running.
Every setting is optional:

```toml
prefix     = "C-a"        # Ctrl + a letter, C-\ C-] C-^ C-_, "F13"–"F24", or '\e[25~'
shell      = "/bin/zsh"   # default: $SHELL
status_bar = true
auto_rename = true        # rename tabs after the active pane's program (manual rename wins)
# random_session_name = false  # true picks <prefix>-<suffix> from 40x40 dictionaries (re-rolls on collision)
scrollback = 5000         # lines kept per pane; 0 turns it off
mouse      = true         # false hands the mouse back to your terminal
restore    = true         # bring the session back after a reboot
animations = true         # false turns every animation off

[agent]                   # how `slat status` reads a pane (see above)
settle      = "750ms"     # quiet for this long after output = idle
input_after = "10s"       # quiet for this long on a prompt = waiting for input

[theme]
name = "catppuccin"       # catppuccin, default, dracula, gruvbox, latte, mono, nord, rose-pine, solarized, tokyo-night
accent = "#fabd2f"        # override any single colour

[borders]
style = "rounded"         # sharp, rounded, heavy, double, dashed, none
# vertical = "┃"          # …or set any glyph yourself

[animation]
split  = "90ms"           # a new pane appearing; 0 turns it off
move   = "120ms"          # two panes trading places
easing = "out-cubic"      # linear, out-quad, out-cubic, out-back
reveal = "curtain"        # curtain or none
glyph  = "░"

[splash]
enabled  = true           # false (or duration = "0s") skips the startup banner
duration = "700ms"
# art    = ["..."]        # custom ASCII/Unicode lines
# subtitle = "slat {version}"
# hint     = "press {prefix} then ? for help"
# fg / sub_fg / bg        # customize splash colours

[dialog]
# fg / sub_fg / head_fg / border_fg / bg   # customize modal dialog & help overlay colours

[finder]
# fg / sub_fg / head_fg / border_fg / bg / sel_fg / sel_bg / match_fg / match_bg  # picker colours

[stack]
# fg / bg / active_fg / active_bg          # stacked pane header colours
# icon = " ▸ "                             # active stacked pane icon (inactive_icon defaults to matching spaces)

[status]
left  = " {workspace} │ {tabs}"
right = "{badge} pane {pane}/{panes}  {time} "
tab   = " {index}:{name}{alert} "
alert = " ●"

[status.badges]
# prefix / help / scroll / copy / move / zoom / stack  # customize or "" to hide {badge} labels

# Example: 3-glyph tabs (icon, 1-based window index, 1-based pane count + zoomed set; 10-entry 0-9 arrays render multi-digit numbers > 9):
# [status]
# left             = "{prefix} {workspace} │ {tabs}"
# right            = "{badge}{workspaces} {pane}/{panes} "
# tab              = " {tab_glyph} {tab_index_glyph} {pane_glyph} {name}{alert} "
# alert            = "{fg=bright-red,bold} ●{/}"
# prefix           = "{fg=cyan,bg=236}  {/}"
# prefix_active    = "{fg=#1e1e2e,bg=#f9e2af,bold}   {/}"
# tab_glyphs       = ["■"]
# tab_zoom_glyph   = "▣"
# tab_index_glyphs         = ["⓪", "①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨"]
# pane_glyphs              = ["⁰", "¹", "²", "³", "⁴", "⁵", "⁶", "⁷", "⁸", "⁹"]
# pane_zoom_glyphs         = ["⓿", "❶", "❷", "❸", "❹", "❺", "❻", "❼", "❽", "❾"]
# current_pane_glyphs      = ["₀", "₁", "₂", "₃", "₄", "₅", "₆", "₇", "₈", "₉"]
# current_pane_id_glyphs   = ["𝟘", "𝟙", "𝟚", "𝟛", "𝟜", "𝟝", "𝟞", "𝟟", "𝟠", "𝟡"]
# tab_colors       = ["cyan", "magenta", "yellow", "green", "blue"]
# pane_colors      = ["208", "44", "#a6e3a1"]

[keybinds]
split-vertical   = "|"
split-horizontal = "-"
resize-shrink    = "_"    # "-" was given away above
zoom             = ""     # "" unbinds a command
```

- `[theme]` picks a built-in palette (loaded from externalized theme TOML files)
  or a custom theme file from `~/.config/slat/themes/<name>.toml` (which can
  add new themes or override built-in ones). Colours are `bg` (alias
  `background`), `status_bg`, `fg` (alias `foreground`), `dim`, `accent`,
  `border`, `tab_bg`, `tab_fg`, `tab_active_bg`, `tab_active_fg`, `prefix_fg`,
  `prefix_bg`, `prefix_active_fg`, `prefix_active_bg`, `badge_fg`, and
  `badge_bg`, written as a colour
  name, a palette index (0-255), `#rrggbb`, or a `theme.<var>` reference
  (`status_bg` and `*_bg` also accept `""` or `"transparent"` for the
  terminal's default background). You can also define custom palette variables
  under `[theme.vars]` (or `[vars]` in a `~/.config/slat/themes/<name>.toml`
  file) and reference them anywhere as `theme.<name>` — including in `[dialog]`,
  `[finder]`, `[stack]`, `[splash]`, `status.tab_colors`/`pane_colors`, and
  status tags like `{fg=theme.background,bg=theme.accent}`. Only what slat draws
  is themed — pane contents keep whatever colours the programs in them use.
- `[status]` takes `left`, `right`, `tab`, `tab_active`, `tab_sep`, `alert`,
  `prefix`, `prefix_active`, `zoom`, `tab_zoom_glyph`, `pane_id`,
  `pane_id_active`, `tab_colors`, `pane_colors`, `tab_glyphs`,
  `tab_index_glyphs`, `tab_index_zoom_glyphs`, `pane_glyphs`,
  `pane_zoom_glyphs`, `current_pane_glyphs`, `current_pane_zoom_glyphs`,
  `current_pane_id_glyphs` and `current_pane_id_zoom_glyphs` (each placeholder
  uses only its own glyph array, with no fallback to `pane_glyphs`), plus
  `[status.zoom_glyphs]` (`tab`, `tab_index`, `pane`, `current_pane`,
  `current_pane_id` — all `false` by default, set to `true` to enable zoomed
  glyph replacement for that glyph type, persisting on inactive zoomed tabs) and `[status.process_glyphs]`
  (a `process = "glyph"` map that overrides `{tab_glyph}` — and populates
  `{process_glyph}` — when the tab's active pane is running that process,
  taking precedence over `tab_zoom_glyph` and `tab_glyphs`). The sides accept `{prefix}` `{session}`
  `{workspace}` `{workspace_index}` `{workspace_count}` `{workspaces}`
  `{tabs}` `{tab}` `{tabs_count}` `{pane}` `{panes}` `{pane_glyph}`
  `{pane_glyph_current}` `{current_pane_glyph}` `{current_pane_id_glyph}`
  `{zoom}` `{badge}` `{time}` `{seconds}` `{date}`; `tab` and `tab_active`
  accept `{index}` `{name}` `{alert}` `{tab_glyph}` `{tab_index_glyph}`
  `{process_glyph}` `{pane_glyph}` `{pane_glyph_current}` `{current_pane_glyph}`
  `{current_pane_id_glyph}` `{zoom}`; `pane_id` and `pane_id_active` (toggled
  in the top-right corner of each pane with `identify-panes`, default `prefix`
  + `i`) accept `{id}` `{index}` `{pane_glyph}` `{current_pane_id_glyph}`; and
  `tab_sep` (default `" "`, set to `""` to remove space between tabs) is drawn
  between tabs. Inline style tags like `{fg=cyan,bg=236,bold}🐟{/}` or `{fg=theme.background,bg=theme.accent}●{/}` colour any Unicode glyph or span (using a colour name, 0–255 index, `#rrggbb`
  hex, an active `[theme]` variable such as `theme.background` / `bg`, `theme.foreground` / `fg`, `theme.accent`, `theme.dim`, `theme.status_bg`, `theme.tab_bg`, `theme.tab_fg`, `theme.tab_active_bg`, `theme.tab_active_fg`, `theme.badge_fg`, `theme.badge_bg`, any `[theme.vars]` key, or `tab`/`pane` to pick from `tab_colors`/`pane_colors` by index). When
  `{prefix}` is used, `{badge}` omits `"PREFIX"`; `"ZOOM"` remains visible in `{badge}` whenever the current tab is zoomed (like `"MOVE"`), and `{badge}` can be styled via `badge_fg`/`badge_bg` or inline style tags.
- Session presets (`~/.config/slat/sessions/<name>.toml`) accept top-level `focus = N` (1-based tab index, or workspace index when `[[workspaces]]` is used), workspace-level `focus = N` (1-based tab index), and `[[tabs]]` `focus = N` (1-based pane index) to choose what starts focused.
- `[status.scripts.<name>]` runs a shell command on an `interval` (default
  `5s`, `0s` to run once) with a `timeout` (default `2s`), and renders its
  `stdout` wherever `{script:<name>}` appears in `status.left` or
  `status.right`. Script output supports `{fg=red}VAL{/}` inline style tags
  (unrecognized `{...}` sequences in stdout are rendered literally) and shows
  `fallback` (default `{fg=red}[err]{/}`, or `""` to hide) on error/timeout.
- `[shortcuts]` binds direct (non-prefix) keys like `"Alt+1"`, `"M-s"`, or
  `"F13"` to shell commands (e.g. `"Alt+1" = "~/.config/slat/scripts/save.sh"`).
  Both shortcut scripts and status scripts receive `SLAT_SESSION`,
  `SLAT_SESSION_DIR`, `SLAT_WORKSPACE`, `SLAT_WORKSPACE_INDEX`,
  `SLAT_WORKSPACE_COUNT`, `SLAT_TAB`, `SLAT_TAB_INDEX`, `SLAT_TAB_COUNT`,
  `SLAT_PANE`, `SLAT_PANE_INDEX`, `SLAT_PANE_COUNT`, `SLAT_PANE_CWD`,
  `SLAT_PANE_FG`, `SLAT_PANE_PID`, `SLAT_PANE_ROWS`, `SLAT_PANE_COLS`,
  `SLAT_PANE_ZOOMED`, and (for shortcuts) `SLAT_STATE_JSON`.
- A mistake anywhere — an unknown colour, a glyph two columns wide, a
  placeholder that doesn't exist, an easing curve that doesn't either — is
  reported when you run `slat`, never quietly drawn.
- **Fonts belong to your terminal, not to slat.** slat draws characters; which
  font paints them is Kitty's, Alacritty's or WezTerm's business. Point that at
  a Nerd Font and its glyphs work here like any other character.
- Ready-made setups live in [`docs/rices/`](docs/rices/) and [`examples/configs/`](examples/configs/) (`minimal`, `catppuccin-studio`, `tokyo-power`, `tokyonight-storm`, `cyberpunk`, `vice-city`, `nord`, `gruvbox-blocks`, `dracula-squares`) — copy one and edit it,
  or send yours in. Example helper scripts for `slat send`, `[shortcuts]`, and `[status.scripts]` live in [`examples/scripts/`](examples/scripts/), and the Neovim extension (`slat.nvim`) lives in [`examples/nvim/slat.nvim/`](examples/nvim/slat.nvim/).
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
cmd/slat/        entry point & mcli-based CLI: starts or attaches to the daemon (run `slat help <cmd>` for per-command flags)
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
website/         the site at noturbob.github.io/slat (Next.js, built by CI)
docs/design/     how the agent CLI was designed
docs/rices/      ready-made configurations, one of which the tests read
packaging/aur/   the AUR package
packaging/apt/   builds the signed apt repository
```

None of `website/`, `docs/` artwork, `packaging/` or `.github/` reaches the
source archive — [`.gitattributes`](.gitattributes) keeps `git archive` (and
so the Debian tarball) down to what actually builds slat.

Releases are built by [GoReleaser](.goreleaser.yaml) when a `v*` tag is pushed:
binaries, `.deb`, `.rpm` and Arch packages for Linux and macOS on amd64 and arm64.
The release then republishes the website and the apt repository to GitHub Pages
([`pages.yml`](.github/workflows/pages.yml)), signed with the `APT_GPG_PRIVATE_KEY` secret.

`make test-fast` runs `go test -short ./...` (sub-second unit tests, skipping slow
PTY and daemon integration tests). `make test` runs `go vet` and the full test
suite with the race detector. The tests in `internal/app` drive real shells
through splits, closes, overlays and workspace changes and check what a
terminal would display.

## Limitations

- A restored session brings back output, not running programs: nothing
  portable can restore a process, so each pane comes back as a fresh shell
  in the same directory.
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
