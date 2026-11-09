#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_fail none "missing -p, --print, -l, --literal, -e, --effective, -d, --depend, -r, --reference, or -a, --agent" "$LITAR"
assert_fail combine "cannot combine print and literal" "$LITAR" -p a -l a
assert_fail combine-rev "cannot combine print and literal" "$LITAR" -l a -p b
assert_fail nofile "missing -f, --file" "$LITAR" -p msg
assert_fail unknown "flag provided but not defined: -t" "$LITAR" -t . -p msg
assert_fail cluster "flag provided but not defined: -al" "$LITAR" -al
assert_fail positional "Usage: litar " "$LITAR" archive.la
assert_fail missing "chunk 'missing' is not defined" "$LITAR" -f archive.la -p missing
assert_fail long "chunk 'missing' is not defined" "$LITAR" --file archive.la --print missing
assert_fail long-eq "chunk 'missing' is not defined" "$LITAR" --file=archive.la -print=missing
assert_text depend-empty '' "$LITAR" -f archive.la -d nope
assert_fail effective "chunk 'missing' is not defined" "$LITAR" -f archive.la -e missing
