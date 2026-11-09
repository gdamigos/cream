#!/usr/bin/env bash
# run one command as several profiles in turn, without switching the global login
# usage: each-profile.sh <profile>... -- <command> [args...]
# example: each-profile.sh work personal -- gh pr list --author @me
set -uo pipefail

profiles=()
while [ $# -gt 0 ] && [ "$1" != "--" ]; do
  profiles+=("$1")
  shift
done
if [ $# -lt 2 ] || [ ${#profiles[@]} -eq 0 ]; then
  echo "usage: $0 <profile>... -- <command> [args...]" >&2
  exit 2
fi
shift

rc=0
for p in "${profiles[@]}"; do
  echo "== $p"
  # keep the last failure's exit code so ci still fails
  cream run "$p" "$@" || rc=$?
done
exit "$rc"
