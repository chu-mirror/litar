#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_ok "design example" design.out \
    "$LITAR" -a -l 'label@:module 1@/chunk' archive.la
assert_ok "long options" design.out \
    "$LITAR" --agent --literal 'label@:module 1@/chunk' archive.la

assert_ok "depend uses the same object" sub1.out \
    "$LITAR" -a -d 'module 1@/substructure 1' archive.la
assert_text "no users is an empty array" $'[]\n' \
    "$LITAR" -a -d 'module 1@/chunk' archive.la
assert_fail "undefined literal stays an error" \
    "chunk 'missing' is not defined" \
    "$LITAR" -a -l missing archive.la

assert_ok "filters keep labels and fill a missing module" filters.out \
    "$LITAR" -a -l seen filters.la

assert_ok "strings escape quotes and backslashes" notes.out \
    "$LITAR" -a -l chunk notes.la
assert_ok "an included block names the file that holds it" include.out \
    "$LITAR" -a -l fromlib withlib.la

assert_text "print stays text" $'line\n' \
    "$LITAR" -a -p chunk notes.la
