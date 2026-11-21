#!/usr/bin/env bash
# Shared helpers for slat sendkeys target scripts.

SLAT_TARGET_FILE="${SLAT_TARGET_FILE:-/tmp/slat.sendkeys.target}"
SLAT_ALT_TARGET_FILE="${SLAT_ALT_TARGET_FILE:-/tmp/slat.alt.sendkeys.target}"

# Resolve a socket path to its human-readable session name, falling back to basename.
slat_resolve_session_name() {
  local sock="$1"
  local name=""

  if command -v jq >/dev/null 2>&1; then
    name="$(slat sessions --json 2>/dev/null \
      | jq -r --arg s "$sock" '.sessions[]? | select(.socket == $s) | .name' \
      | head -n1 || true)"
  elif command -v python3 >/dev/null 2>&1; then
    name="$(slat sessions --json 2>/dev/null \
      | python3 -c 'import json,sys; s=sys.argv[1]; d=json.load(sys.stdin); print(next((x.get("name","") for x in d.get("sessions",[]) if x.get("socket")==s),""))' "$sock" 2>/dev/null || true)"
  fi

  if [[ -n "$name" ]]; then
    printf '%s' "$name"
  else
    basename "$sock" .sock
  fi
}

# Format a target string (e.g. "/run/.../slat.sock:2" or "work:2") for display.
# Pass "omit_local" as $2 to strip the session prefix when it matches the current session.
slat_format_target() {
  local input="$1"
  local mode="${2:-}"

  if [[ "$input" != *:* ]]; then
    printf '%s' "$input"
    return
  fi

  local key="${input%%:*}"
  local val="${input#*:}"

  if [[ "$mode" == "omit_local" ]] && [[ "$key" == "${SLAT_SESSION:-}" || "$key" == "${SLAT:-}" ]]; then
    printf '%s' "$val"
    return
  fi

  if [[ "$key" == */*.sock ]]; then
    key="$(slat_resolve_session_name "$key")"
  fi

  printf '%s:%s' "$key" "$val"
}

# Read a target file, stripping whitespace; prints nothing if missing/unreadable.
slat_read_target() {
  local path="$1"
  [[ -r "$path" ]] || return 0
  tr -d '[:space:]' < "$path"
}

# Write content atomically so status scripts never observe a truncated file.
slat_write_file() {
  local path="$1"
  local content="$2"
  local tmp
  tmp="$(mktemp "${path}.XXXXXX")"
  printf '%s\n' "$content" > "$tmp"
  mv -f "$tmp" "$path"
}

slat_notify() {
  local title="$1"
  local body="$2"
  if command -v dunstify >/dev/null 2>&1; then
    dunstify "$title" "$body" || true
  elif command -v notify-send >/dev/null 2>&1; then
    notify-send "$title" "$body" || true
  fi
}
