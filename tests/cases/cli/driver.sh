#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

check_help() {
    _name=$1
    shift
    _out=$(mktemp)
    _err=$(mktemp)
    set +e
    "$@" >"$_out" 2>"$_err"
    _st=$?
    set -e
    if [ "$_st" -ne 0 ]; then
        echo "FAIL $_name: exit $_st" >&2
        cat "$_err" >&2
        exit 1
    fi
    if [ -s "$_err" ]; then
        echo "FAIL $_name: stderr" >&2
        cat "$_err" >&2
        exit 1
    fi
    for _needle in "Usage:" "--help" "-p, --print EXPRESSION" \
        "label1@:label2@:module name@/chunk name@|filter1@|filter2" \
        "$LITAR --help"; do
        if ! grep -F -q -e "$_needle" "$_out"; then
            echo "FAIL $_name: missing [$_needle]" >&2
            cat "$_out" >&2
            exit 1
        fi
    done
    rm -f "$_out" "$_err"
}

check_help "help" "$LITAR" --help
check_help "help wins over -p" "$LITAR" --help -p
check_help "help ignores a missing archive" "$LITAR" --help no-such.la

assert_text "-p" $'OK\n' "$LITAR" -p chunk archive.la
assert_text "--print" $'OK\n' "$LITAR" --print chunk archive.la
assert_text "--print=" $'OK\n' "$LITAR" --print=chunk archive.la
assert_text "archive before -p" $'OK\n' "$LITAR" archive.la -p chunk

assert_fail "duplicate -p and --print" "duplicate print option" \
    "$LITAR" -p chunk --print chunk archive.la
assert_fail "duplicate --print=" "duplicate print option" \
    "$LITAR" --print=chunk -p chunk archive.la
assert_fail "unknown long option" "unknown option '--bogus'" \
    "$LITAR" --bogus archive.la
assert_fail "unknown short option" "unknown option '-z'" \
    "$LITAR" -z archive.la
assert_fail "missing archive" "missing archive" "$LITAR" -p chunk
assert_fail "missing print" "missing -p or --print" "$LITAR" archive.la
assert_fail "--print needs an expression" "--print requires an expression" \
    "$LITAR" --print
assert_fail "-p needs an expression" "-p requires an expression" \
    "$LITAR" -p
assert_fail "extra argument" "unexpected argument 'extra'" \
    "$LITAR" -p chunk archive.la extra
assert_fail "missing file" "could not open" "$LITAR" -p chunk no-such.la
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
assert_fail "directory is not an archive" "could not read" \
    "$LITAR" -p chunk "$dir"
assert_fail "empty expression" "empty chunk name" \
    "$LITAR" --print= archive.la
assert_fail "filter in expression" "filter 'filt' is not defined in expression" \
    "$LITAR" -p 'chunk@|filt' archive.la
