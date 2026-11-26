#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=slat-target-lib.sh
source "${SCRIPT_DIR}/slat-target-lib.sh"

input="$(slat_read_target "$SLAT_TARGET_FILE")"
[[ -n "$input" ]] || exit 0

formatted="$(slat_format_target "$input" omit_local)"
printf '{fg=tab} 🖪 %s{/} ' "$formatted"
