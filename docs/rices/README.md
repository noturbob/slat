# Rices

Complete configurations you can copy over `~/.config/slat/config.toml` and
edit. Each one only sets what it changes; everything else keeps its default.

slat draws its own furniture — the status bar, the lines between panes, the
overlays — and all of it is yours: colours, glyphs, the bar's layout, the
animations, every key. What slat can't change is the **font**: that belongs to
your terminal (Kitty, Alacritty, WezTerm, Windows Terminal). Point that at a
Nerd Font and its glyphs work here like any other character.

| Rice | What it is |
| --- | --- |
| [`default.toml`](default.toml) | Everything slat ships with, written out, as a starting point |
| [`powerline.toml`](powerline.toml) | Nord colours, chevron separators, a clock. Needs a Nerd Font |
| [`minimal.toml`](minimal.toml) | Tab numbers and a pane counter, nothing else, no animation |
| [`tty.toml`](tty.toml) | 16 colours and ASCII borders, for a bare Linux console or `ssh` to something old |

These bars are rendered by slat itself, so they are what you get:

```
default     dev │  1:shell   2:build ?   3:logs            PREFIX  ws 1/2 · pane 2/3
powerline   dev  1 shell   2 build ●   3 logs                     PREFIX  2/3  12:01
minimal     1  2!  3                                                          2/3
tty         dev | 1:shell  2:build !  3:logs                          pane 2/3
```

## Sending one in

Pull requests welcome: add a `.toml` here with a comment at the top saying what
it needs (a Nerd Font, true colour, …) and a row in the table above. Keep it to
settings — a rice that only works with a patched build isn't one.

## What each setting does

See the [configuration section of the README](../../README.md#configuration),
`man slat`, or [`config.example.toml`](../../config.example.toml), which lists
every option with its default.
