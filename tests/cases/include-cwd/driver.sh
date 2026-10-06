#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
cd "$dir"
assert_fail "search starts at cwd" "could not find archive 'lib/light.la'" \
    "$LITAR" -p hello.c "$ROOT/examples/hello.la"
