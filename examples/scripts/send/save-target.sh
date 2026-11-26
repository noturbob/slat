#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=slat-target-lib.sh
source "${SCRIPT_DIR}/slat-target-lib.sh"

: "${SLAT:?SLAT is not set}"
: "${SLAT_PANE:?SLAT_PANE is not set}"

id="${SLAT}:${SLAT_PANE}"
slat_write_file "$SLAT_TARGET_FILE" "$id"
slat_write_file "$SLAT_ALT_TARGET_FILE" "$id"

display_id="$(slat_format_target "$id")"
slat_notify "slat" "Saved target '${display_id}'"
