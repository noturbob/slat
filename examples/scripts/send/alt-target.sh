#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=slat-target-lib.sh
source "${SCRIPT_DIR}/slat-target-lib.sh"

# Prefer $SLAT (unique socket path) over $SLAT_SESSION so sessions with the
# same directory basename never collide.
session_ref="${SLAT:-${SLAT_SESSION:-}}"
if [[ -z "$session_ref" || -z "${SLAT_PANE:-}" ]]; then
  slat display "{fg=red,bold}Missing slat context{/}"
  exit 1
fi
current="${session_ref}:${SLAT_PANE}"

target="$(slat_read_target "$SLAT_ALT_TARGET_FILE")"
if [[ -z "$target" ]]; then
  slat display "{fg=red,bold}No target, use alt-1 to save a new one{/}"
  exit 0
fi

if [[ "$target" == "$current" ]]; then
  slat display "{fg=dim}Already on target pane{/}"
  exit 0
fi

if slat focus "$target" 2>/dev/null; then
  slat_write_file "$SLAT_ALT_TARGET_FILE" "$current"
else
  slat display "{fg=red,bold}Target not found: $(slat_format_target "$target"){/}"
fi
