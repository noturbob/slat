# slat zsh extension: fzf picker for preconfigured sessions in ~/.config/slat/sessions/
# Source in ~/.zshrc:
#   source /path/to/slat/shell/slat.zsh

slat-session() {
  local sessions_dir="${XDG_CONFIG_HOME:-$HOME/.config}/slat/sessions"
  if [[ ! -d "$sessions_dir" ]]; then
    print -u2 "slat-session: no sessions directory at $sessions_dir"
    return 1
  fi

  local -a files
  files=("$sessions_dir"/*.toml(N:t:r))
  if (( ${#files[@]} == 0 )); then
    print -u2 "slat-session: no .toml sessions found in $sessions_dir"
    return 1
  fi

  local choice
  choice=$(
    printf '%s\n' "${files[@]}" \
      | sort \
      | fzf --prompt="slat session> " \
            --preview="cat '$sessions_dir/{}.toml'" \
            --preview-window=right:50%:wrap
  ) || return 0

  if [[ -n "$choice" ]]; then
    slat session "$choice"
  fi
}
