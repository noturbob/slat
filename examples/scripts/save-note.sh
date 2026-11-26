#!/bin/sh
# Example [shortcuts] script for "Alt+1" = "~/.config/slat/scripts/save-note.sh"
#
# Logs the active session, workspace, tab, pane ID, and working directory
# using the SLAT_* environment variables provided by slat, and optionally
# forwards a status line to a dedicated "logs" session/pane via `slat send`.

set -eu

LOG_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/slat"
mkdir -p "$LOG_DIR"
LOG_FILE="$LOG_DIR/notes.log"

STAMP=$(date '+%Y-%m-%d %H:%M:%S')
ENTRY="$STAMP session=${SLAT_SESSION:-?} ws=${SLAT_WORKSPACE:-?} tab=${SLAT_TAB_INDEX:-?}:${SLAT_TAB:-?} pane=${SLAT_PANE:-?} cwd=${SLAT_PANE_CWD:-?}"
printf '%s\n' "$ENTRY" >> "$LOG_FILE"

# Optional: if a "notes" session exists, echo the snapshot into its active pane.
if slat sessions --json 2>/dev/null | grep -q '"name":"notes"'; then
  slat send notes:active "echo '$ENTRY'" --enter >/dev/null 2>&1 || true
fi
