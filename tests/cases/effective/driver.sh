#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_text "set then extend" \
    $'@<chunk1@=\nThe contents of block1.\n@\n\n@<chunk1@+\nThe contents of block3.\n@\n' \
    "$LITAR" -e chunk1 archive.la
assert_text "later set drops the earlier block" \
    $'@<chunk2@=\nThe contents of block4.\n@\n' \
    "$LITAR" -e chunk2 archive.la
assert_text "literal still prints the dropped block" \
    $'@<chunk2@=\nThe contents of block2.\n@\n\n@<chunk2@=\nThe contents of block4.\n@\n' \
    "$LITAR" -l chunk2 archive.la

assert_text "extend keeps the predecessor" \
    $'\nKept by a later extension.\n@<base@=\nA\n@\n\n@<spec@:base@+\nB\n@\n' \
    "$LITAR" -e 'spec@:base' archive.la
assert_text "set drops the predecessor" \
    $'\nReplaced predecessor.\n@<spec@:gone@=\nnew\n@\n' \
    "$LITAR" -e 'spec@:gone' archive.la
assert_text "unspecialized set stays" \
    $'@<gone@=\nold\n@\n' \
    "$LITAR" -e gone archive.la
assert_text "later set drops an earlier extend" \
    $'@<late@=\nC\n@\n' \
    "$LITAR" -e late archive.la
assert_text "less specialized set drops an earlier extend" \
    $'@<early@=\ny\n@\n' \
    "$LITAR" -e 'over@:early' archive.la

assert_text "named module keeps its own chain" \
    $'\nLater set in Left.\n@<Left@/item@=\nonly\n@\n' \
    "$LITAR" -e 'Left@/item' archive.la
assert_text "another module is not dropped" \
    $'@<Right@/item@=\nright\n@\n\n@<Right@/item@+\nmore\n@\n' \
    "$LITAR" -e 'Right@/item' archive.la
assert_text "no module lists each module's chain" \
    $'@<Right@/item@=\nright\n@\n\n@<Right@/item@+\nmore\n@\n\nLater set in Left.\n@<Left@/item@=\nonly\n@\n' \
    "$LITAR" -e item archive.la

assert_text "filters stay as written" \
    $'@<word@=\nab\n@|up@\n\n@<word@+\ncd\n@\n' \
    "$LITAR" -e word archive.la
assert_text "print still runs the filter" \
    $'AB\ncd\n' \
    "$LITAR" -p word archive.la

assert_text "long option" \
    $'@<chunk2@=\nThe contents of block4.\n@\n' \
    "$LITAR" --effective chunk2 archive.la
assert_text "--effective=" \
    $'@<chunk2@=\nThe contents of block4.\n@\n' \
    "$LITAR" --effective=chunk2 archive.la

agent=$'[\n    {\n        "comment": "",\n        "labels": [],\n        "module": "",\n        "chunk": "chunk2",\n        "type": "set",\n        "content": "The contents of block4.\\n",\n        "filters": [],\n        "file": "archive.la",\n        "line-range": [12, 15]\n    }\n]\n'
assert_text "agent listing" "$agent" "$LITAR" -a -e chunk2 archive.la

assert_fail "undefined" "chunk 'missing' is not defined" \
    "$LITAR" -e missing archive.la
assert_fail "filters are not part of a chunk reference" \
    "unexpected control '@|' in name" \
    "$LITAR" -e 'word@|up' archive.la
assert_fail "empty reference" "empty chunk name" \
    "$LITAR" --effective= archive.la
