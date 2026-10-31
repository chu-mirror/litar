#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text modules $'a\na@/b\n' "$LITAR" -t . -m /
assert_text under-a $'a@/b\n' "$LITAR" -t . -m a
assert_text chunks $'z\nhello.c\n' "$LITAR" -t . -c /
assert_text chunk-ab $'c\n' "$LITAR" -t . -c 'a@/b'
assert_text chunk-a $'d\n' "$LITAR" -t . -c a
assert_text sub $'C\nonly\n' "$LITAR" -t . -s /
assert_text sub-c $'C@/variables\nC@/functions\n' "$LITAR" -t . -s C
assert_text items $'only\n' "$LITAR" -t . -i /
assert_text item-cv $'C@/variables@/times\n' "$LITAR" -t . -i 'C@/variables'
assert_text refer $'@<z@=\nq\n@~C@/variables@/times@~C@/functions@/main@~only@\n' \
    "$LITAR" -t . -r 'C@/functions@/main'
assert_text absent '' "$LITAR" -t . -m nope
assert_text no-item '' "$LITAR" -t . -r missing
