#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_text "set then extend" $'The contents of block1.\nThe contents of block3.\n' \
    "$LITAR" -p chunk1 archive.la
assert_text "later set overrides" $'The contents of block4.\n' \
    "$LITAR" -p chunk2 archive.la
assert_text "extend keeps the predecessor" $'A\nB\n' \
    "$LITAR" -p 'spec@:base' archive.la
assert_text "set replaces the predecessor" $'new\n' \
    "$LITAR" -p 'spec@:gone' archive.la
assert_text "unspecialized set stays" $'old\n' \
    "$LITAR" -p gone archive.la
assert_text "extend appends filtered text" $'AB\ncd\n' \
    "$LITAR" -p word archive.la
assert_text "literal keeps each operator" \
    $'@<chunk1@=\nThe contents of block1.\n@\n\n@<chunk1@+\nThe contents of block3.\n@\n' \
    "$LITAR" -l chunk1 archive.la
