#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$ROOT"
expected=$(mktemp)
trap 'rm -f "$expected"' EXIT
which perl >"$expected"
assert_ok "path to perl interpreter" "$expected" \
    "$LITAR" -p "Text Processing@/path to perl interpreter" \
    "$ROOT/lib/text-processing.la"
path=$(tr -d '\n' <"$expected")
if [ ! -x "$path" ]; then
    echo "FAIL perl path is not executable: $path" >&2
    exit 1
fi
