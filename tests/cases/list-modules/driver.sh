#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

modules=$'\nEarly\nShown\nOther\nFrom lib\n'

assert_text "introduced modules" "$modules" "$LITAR" -m archive.la
assert_text "--modules matches -m" "$modules" "$LITAR" --modules archive.la

assert_text "early module" $'e\n' "$LITAR" -p 'Early@/first' archive.la
assert_text "kept arm" $'yes\n' "$LITAR" -p 'Shown@/piece' archive.la
assert_text "header module" $'x\n' "$LITAR" -p 'Other@/via header' archive.la
assert_text "included module" $'lib\n' "$LITAR" -p 'From lib@/lib chunk' archive.la

assert_fail "skipped module is absent" "chunk 'Hidden@/secret' is not defined" \
    "$LITAR" -p 'Hidden@/secret' archive.la
assert_fail "reference does not introduce a module" \
    "chunk 'OnlyRef@/nope' is not defined" \
    "$LITAR" -l 'OnlyRef@/nope' archive.la

assert_fail "print still rejects the cycle" "circular inclusion of 'cycle'" \
    "$LITAR" -p cycle archive.la
assert_text "modules ignores a cycle" "$modules" "$LITAR" -m archive.la

assert_fail "print still runs the filter" "filter 'missing-filter' is not defined" \
    "$LITAR" -p bad archive.la
assert_text "modules does not run filters" "$modules" "$LITAR" -m archive.la

assert_fail "parse error lists nothing" "unterminated block" \
    "$LITAR" -m bad.la
