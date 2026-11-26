#!/bin/sh
# Example: send commands to panes in other sessions, workspaces, or tabs.
#
# Usage:
#   ./send-to-pane.sh <target> <command...>
#
# Examples:
#   ./send-to-pane.sh work:2 "git status"
#   ./send-to-pane.sh work:dev/editor/active "make test"
#   ./send-to-pane.sh dev/1/active "ls -la"

set -eu

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <target> <command...>" >&2
  echo "  target: [SESSION:][WORKSPACE/[TAB/]]PANE (e.g. work:2 or work:dev/1/active)" >&2
  exit 1
fi

TARGET=$1
shift

# You can also pass --session, --workspace, and --tab explicitly:
#   slat send --session work --workspace dev --tab 1 active "make test" --enter
slat send "$TARGET" "$*" --enter
