#!/usr/bin/env bash
# switch to a profile only when it isn't active yet, then fail if a tool is still on another account
# usage: ensure-profile.sh <profile>
# needs jq
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <profile>" >&2
  exit 2
fi
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
profile=$1

status=$(cream status --json)
if jq -e --arg p "$profile" '.locked.all or ((.locked.profiles // []) | index($p) != null)' <<<"$status" >/dev/null; then
  echo "$profile is locked, run \`cream unlock\` first" >&2
  exit 1
fi

if [ "$(jq -r .active <<<"$status")" != "$profile" ]; then
  cream use "$profile"
  status=$(cream status --json)
fi

# tools whose live login differs from the account saved in the profile
wrong=$(jq -r '.adapters[]
  | select(.expected != null and (.match | not))
  | "  \(.adapter): \(.account // "not logged in"), expected \(.expected)"' <<<"$status")

if [ -n "$wrong" ]; then
  echo "not every tool is on $profile:" >&2
  echo "$wrong" >&2
  echo "run \`cream doctor\` for fixes" >&2
  exit 1
fi
echo "on $profile"
