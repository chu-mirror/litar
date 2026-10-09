#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
out=$(mktemp)
err=$(mktemp)
exp_out=$(mktemp)
exp_err=$(mktemp)
trap 'rm -f "$out" "$err" "$exp_out" "$exp_err"' EXIT
set +e
"$LITAR" -p c archive.la >"$out" 2>"$err"
st=$?
set -e
if [ "$st" -ne 0 ]; then
    echo "FAIL filter stderr: exit $st" >&2
    cat "$err" >&2
    exit 1
fi
printf 'out\n' >"$exp_out"
printf 'e\n' >"$exp_err"
if ! cmp -s "$exp_out" "$out"; then
    echo "FAIL filter stderr: stdout" >&2
    diff -u "$exp_out" "$out" >&2 || true
    exit 1
fi
if ! cmp -s "$exp_err" "$err"; then
    echo "FAIL filter stderr: stderr" >&2
    diff -u "$exp_err" "$err" >&2 || true
    exit 1
fi
