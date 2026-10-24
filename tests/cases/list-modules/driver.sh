#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

top=$'Early\nShown\nOther\nDeep\nFrom lib\n'
assert_text "modules under the root" "$top" "$LITAR" -m / archive.la
assert_text "--modules matches -m" "$top" "$LITAR" --modules / archive.la
assert_text "--modules=" "$top" "$LITAR" --modules=/ archive.la
assert_text "root may be omitted" "$top" "$LITAR" -m '' archive.la

assert_text "children of a module" $'Deep@/Nested\n' \
    "$LITAR" -m Deep archive.la
assert_text "a leaf module has no children" "" \
    "$LITAR" -m 'Deep@/Nested' archive.la
assert_text "an unknown module has no children" "" \
    "$LITAR" -m Nope archive.la

assert_text "chunks under the root" $'cycle\nbad\nchunk\n' \
    "$LITAR" -c / archive.la
assert_text "chunks of a module" $'piece\n' \
    "$LITAR" -c Shown archive.la
assert_text "a submodule owns its chunks" "" \
    "$LITAR" -c Deep archive.la
assert_text "chunks of a nested module" $'part\n' \
    "$LITAR" -c 'Deep@/Nested' archive.la
assert_text "--chunks" $'piece\n' \
    "$LITAR" --chunks Shown archive.la

assert_text "early module" $'e\n' "$LITAR" -p 'Early@/first' archive.la
assert_text "kept arm" $'yes\n' "$LITAR" -p 'Shown@/piece' archive.la
assert_text "header module" $'x\n' "$LITAR" -p 'Other@/via header' archive.la
assert_text "included module" $'lib\n' \
    "$LITAR" -p 'From lib@/lib chunk' archive.la

assert_fail "skipped module is absent" "chunk 'Hidden@/secret' is not defined" \
    "$LITAR" -p 'Hidden@/secret' archive.la
assert_fail "reference does not introduce a module" \
    "chunk 'OnlyRef@/nope' is not defined" \
    "$LITAR" -l 'OnlyRef@/nope' archive.la

assert_fail "print still rejects the cycle" "circular inclusion of 'cycle'" \
    "$LITAR" -p cycle archive.la
assert_text "modules ignores a cycle" "$top" "$LITAR" -m / archive.la

assert_fail "print still runs the filter" "filter 'missing-filter' is not defined" \
    "$LITAR" -p bad archive.la
assert_text "modules does not run filters" "$top" "$LITAR" -m / archive.la

assert_fail "parse error lists nothing" "unterminated block" \
    "$LITAR" -m / bad.la
