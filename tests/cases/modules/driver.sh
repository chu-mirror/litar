#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text modules $'a\na@/b\n' "$LITAR" -f composer.json -m /
assert_text under-a $'a@/b\n' "$LITAR" -f composer.json -m a
assert_text chunks $'z\nhello.c\n' "$LITAR" -f composer.json -c /
assert_text chunk-ab $'c\n' "$LITAR" -f composer.json -c 'a@/b'
assert_text chunk-a $'d\n' "$LITAR" -f composer.json -c a
assert_text sub $'C\nonly\n' "$LITAR" -f composer.json -s /
assert_text sub-c $'C@/variables\nC@/functions\n' "$LITAR" -f composer.json -s C
assert_text items $'only\n' "$LITAR" -f composer.json -i /
assert_text item-cv $'C@/variables@/times\n' "$LITAR" -f composer.json -i 'C@/variables'
assert_text refer $'@<z@=\nq\n@~C@/variables@/times@~C@/functions@/main@~only@\n' \
    "$LITAR" -f composer.json -r 'C@/functions@/main'
assert_text absent '' "$LITAR" -f composer.json -m nope
assert_text no-item '' "$LITAR" -f composer.json -r missing
