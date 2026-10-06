#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
here=$(cd "$(dirname "$0")" && pwd)
src=$(mktemp)
err=$(mktemp)
bin=$(mktemp)
run=$(mktemp)
trap 'rm -f "$src" "$err" "$bin" "$run"' EXIT
if ! (cd "$ROOT" && "$LITAR" -p hello.c examples/hello.la >"$src" 2>"$err"); then
    echo "FAIL hello extract" >&2
    cat "$err" >&2
    exit 1
fi
if [ -s "$err" ]; then
    echo "FAIL hello stderr" >&2
    cat "$err" >&2
    exit 1
fi
if ! cmp -s "$here/out" "$src"; then
    echo "FAIL hello bytes" >&2
    diff -u "$here/out" "$src" >&2 || true
    exit 1
fi
if ! gcc -x c -o "$bin" "$src"; then
    echo "FAIL hello compile" >&2
    exit 1
fi
if ! "$bin" >"$run"; then
    echo "FAIL hello run" >&2
    exit 1
fi
printf 'Hello\n' >"$err"
if ! cmp -s "$err" "$run"; then
    echo "FAIL hello output" >&2
    diff -u "$err" "$run" >&2 || true
    exit 1
fi
