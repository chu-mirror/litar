#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text msg Hello "$LITAR" --file=archive.la -p msg
assert_text line Hi "$LITAR" -f archive.la -p line
assert_text nest '!' "$LITAR" -f archive.la -p base
assert_text attached Hello "$LITAR" -farchive.la -pmsg
