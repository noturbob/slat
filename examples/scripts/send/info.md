This is a set of scripts to enable sending over panes over sessions.
To be used with combination of neovim plugin.

```toml
[status.scripts.sendkeys]
command  = "bash ~/.config/slat/scripts/status-target.sh"
interval = "1s"            # how often to re-run (default 5s; 0s = run once)
timeout  = "1s"            # max run time before killing (default 2s)
fallback = "{fg=red}[err]{/}" # shown on error/timeout ("" hides on error)

[shortcuts]
"Alt+1" = "~/.config/slat/scripts/save-target.sh"
"Alt+2" = "~/.config/slat/scripts/alt-target.sh"
```

And use in status line:
```toml
right         = "{badge} {script:sendkeys}{time} "
```
