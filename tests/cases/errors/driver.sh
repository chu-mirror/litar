#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_fail none "missing -p, --print, -l, --literal, -e, --effective, -d, --depend, -m, --modules, -c, --chunks, -s, --subdomains, -i, --items, -r, --reference, or -a, --agent" "$LITAR"
assert_fail combine "cannot combine print and literal" "$LITAR" -p a -l a
assert_fail combine-rev "cannot combine print and literal" "$LITAR" -l a -p b
assert_fail dup "duplicate print option" "$LITAR" -p a -p b
assert_fail nofile "missing -f, --file" "$LITAR" -p msg
assert_fail cluster "unknown option '-al'" "$LITAR" -al
assert_fail positional "unexpected argument 'archive.la'" "$LITAR" archive.la
assert_fail missing "chunk 'missing' is not defined" "$LITAR" -f composer.json -p missing
assert_text depend-empty '' "$LITAR" -f composer.json -d nope
assert_text modules-empty '' "$LITAR" -f composer.json -m /
assert_fail effective "chunk 'missing' is not defined" "$LITAR" -f composer.json -e missing
