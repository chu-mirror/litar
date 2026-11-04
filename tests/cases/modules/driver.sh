#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text refer $'@<z@=\nq\n@~C@/variables@/times@~C@/functions@/main@~only@\n' \
    "$LITAR" -f archive.la -r 'C@/functions@/main'
assert_text no-item '' "$LITAR" -f archive.la -r missing
