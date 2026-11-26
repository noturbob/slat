# slat bash extension: fzf picker for preconfigured sessions in ~/.config/slat/sessions/
# Source in ~/.bashrc:
#   source /path/to/slat/shell/slat.bash

slat-session() {
  local sessions_dir="${XDG_CONFIG_HOME:-$HOME/.config}/slat/sessions"
  if [[ ! -d "$sessions_dir" ]]; then
    echo "slat-session: no sessions directory at $sessions_dir" >&2
    return 1
  fi

  local choice
  choice=$(
    find "$sessions_dir" -maxdepth 1 -type f -name '*.toml' -printf '%f\n' 2>/dev/null \
      | sed 's/\.toml$//' \
      | sort \
      | fzf --prompt="slat session> " \
            --preview="cat '$sessions_dir/{}.toml'" \
            --preview-window=right:50%:wrap
  ) || return 0

  if [[ -n "$choice" ]]; then
    slat session "$choice"
  fi
}
