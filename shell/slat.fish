# slat fish extension: fzf picker for preconfigured sessions in ~/.config/slat/sessions/
# Source in ~/.config/fish/config.fish:
#   source /path/to/slat/shell/slat.fish

function slat-session --description "Pick a preconfigured slat session with fzf"
    set -l config_home $XDG_CONFIG_HOME
    if test -z "$config_home"
        set config_home "$HOME/.config"
    end
    set -l sessions_dir "$config_home/slat/sessions"

    if not test -d "$sessions_dir"
        echo "slat-session: no sessions directory at $sessions_dir" >&2
        return 1
    end

    set -l files $sessions_dir/*.toml
    if test (count $files) -eq 0
        echo "slat-session: no .toml sessions found in $sessions_dir" >&2
        return 1
    end

    set -l choice (
        for f in $files
            basename "$f" .toml
        end | sort | fzf --prompt="slat session> " \
                         --preview="cat '$sessions_dir/{}.toml'" \
                         --preview-window=right:50%:wrap
    )

    if test -n "$choice"
        slat session "$choice"
    end
end
