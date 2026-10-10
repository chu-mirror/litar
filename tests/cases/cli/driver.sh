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
    for _needle in "usage: $LITAR [options] ARCHIVE" \
        "--help: print the usage" \
        "-p, --print EXPRESSION: evaluate EXPRESSION, and print it" \
        "-l, --literal CHUNK_REFERENCE: print the blocks extending CHUNK_REFERENCE literally" \
        "label1@:label2@:module name@/chunk name@|filter1@|filter2" \
        "label1@:label2@:module name@/chunk name."; do
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
check_help "help wins over -l" "$LITAR" --help -l
check_help "help ignores a missing archive" "$LITAR" --help no-such.la

assert_text "-p" $'OK\n' "$LITAR" -p chunk archive.la
assert_text "--print" $'OK\n' "$LITAR" --print chunk archive.la
assert_text "--print=" $'OK\n' "$LITAR" --print=chunk archive.la
assert_text "archive before -p" $'OK\n' "$LITAR" archive.la -p chunk

assert_text "-l" $'OK\n' "$LITAR" -l chunk archive.la
assert_text "--literal" $'OK\n' "$LITAR" --literal chunk archive.la
assert_text "--literal=" $'OK\n' "$LITAR" --literal=chunk archive.la
assert_text "archive before -l" $'OK\n' "$LITAR" archive.la -l chunk

assert_fail "duplicate -p and --print" "duplicate print option" \
    "$LITAR" -p chunk --print chunk archive.la
assert_fail "duplicate --print=" "duplicate print option" \
    "$LITAR" --print=chunk -p chunk archive.la
assert_fail "duplicate -l and --literal" "duplicate literal option" \
    "$LITAR" -l chunk --literal chunk archive.la
assert_fail "duplicate --literal=" "duplicate literal option" \
    "$LITAR" --literal=chunk -l chunk archive.la
assert_fail "print and literal" "cannot combine print and literal" \
    "$LITAR" -p chunk -l chunk archive.la
assert_fail "literal and print" "cannot combine print and literal" \
    "$LITAR" --literal chunk --print chunk archive.la
assert_fail "unknown long option" "unknown option '--bogus'" \
    "$LITAR" --bogus archive.la
assert_fail "unknown short option" "unknown option '-z'" \
    "$LITAR" -z archive.la
assert_fail "missing archive" "missing archive" "$LITAR" -p chunk
assert_fail "missing print" "missing -p, --print, -l, or --literal" \
    "$LITAR" archive.la
assert_fail "--print needs an expression" "--print requires an expression" \
    "$LITAR" --print
assert_fail "-p needs an expression" "-p requires an expression" \
    "$LITAR" -p
assert_fail "--literal needs a chunk reference" \
    "--literal requires a chunk reference" "$LITAR" --literal
assert_fail "-l needs a chunk reference" "-l requires a chunk reference" \
    "$LITAR" -l
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
