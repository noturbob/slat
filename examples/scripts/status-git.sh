#!/bin/sh
# Example [status.scripts.git] script.
# Outputs the current git branch for the active pane's working directory,
# coloured with slat's inline {fg=...}...{/} style tags (including theme colours).

set -eu

DIR=${SLAT_PANE_CWD:-$PWD}
BRANCH=$(git -C "$DIR" branch --show-current 2>/dev/null || true)
if [ -z "$BRANCH" ]; then
  exit 0
fi

if [ -n "$(git -C "$DIR" status --porcelain 2>/dev/null)" ]; then
  printf '{fg=red,bold}git:%s*{/}' "$BRANCH"
else
  printf '{fg=accent}git:%s{/}' "$BRANCH"
fi
