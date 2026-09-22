# The README demo, recorded by vhs (https://github.com/charmbracelet/vhs):
#
#     go install github.com/charmbracelet/vhs@latest   # needs ttyd + ffmpeg
#     make demo                                        # or: vhs docs/demo.tape
#
# It records the real program, so it is regenerated rather than re-shot
# whenever slat's look changes. The session it builds is the one the README
# describes: split, run two things, copy out of the scrollback, detach,
# reattach with everything still alive.

Output docs/demo.gif

Require slat

Set Shell bash
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 24
Set Margin 24
Set BorderRadius 10
Set WindowBar Colorful
Set Framerate 24
Set PlaybackSpeed 1.0
Set TypingSpeed 55ms

# slat's own colours: canvas #08090a, xterm 252 text, xterm 44 for the
# focused border — the same values internal/ui/theme.go paints with.
Set Theme { "background": "#08090a", "foreground": "#d0d0d0", "cursor": "#00d7d7", "selection": "#1c3b3b", "black": "#08090a", "red": "#c05050", "green": "#5faf5f", "yellow": "#d7af5f", "blue": "#2b6cb0", "magenta": "#a06cc0", "cyan": "#00d7d7", "white": "#d0d0d0", "brightBlack": "#6b6f74", "brightRed": "#d07070", "brightGreen": "#87d787", "brightYellow": "#ffd787", "brightBlue": "#5f87d7", "brightMagenta": "#c087d7", "brightCyan": "#5fffff", "brightWhite": "#f2f2f2" }

Hide
Type "export PS1='$ '" Enter
Type "clear" Enter
Show

Sleep 500ms
Type "slat" Enter
Sleep 2s

# Split left/right, then the right half top/bottom.
Ctrl+s Type "v" Sleep 1s
Ctrl+s Type "h" Sleep 1s

# Two things running at once, in their own panes.
Type "watch -n1 date" Enter Sleep 2500ms
Ctrl+s Type "k" Sleep 700ms
Type "go test ./internal/vt/" Enter Sleep 4s

# Back to the first pane; print something worth copying.
Ctrl+s Type "H" Sleep 700ms
Type "go version && uname -sr" Enter Sleep 1500ms

# Scroll back, select a line, yank it to the system clipboard.
Ctrl+s Type "[" Sleep 800ms
Type "kk" Sleep 600ms
Type "V" Sleep 800ms
Type "y" Sleep 1500ms

# Detach: the shells keep running without a terminal attached.
Ctrl+s Type "d" Sleep 1500ms
Type "slat ls" Enter Sleep 2s

# Reattach: everything is where it was, output included.
Type "slat" Enter Sleep 3s

Hide
Ctrl+s Type "q"
Sleep 1s
