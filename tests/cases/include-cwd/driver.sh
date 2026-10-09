#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
dir=$(mktemp -d)
archive=$(mktemp)
trap 'rm -rf "$dir" "$archive"' EXIT
printf '%s\n' '@. lib/light.la @' '@<c@=' 'x' '@' >"$archive"
cd "$dir"
assert_fail "search starts at cwd" "could not find archive 'lib/light.la'" \
    "$LITAR" -p c "$archive"
