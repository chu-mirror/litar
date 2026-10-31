#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_ok literal literal.out "$LITAR" -t literal -l 'label@:module@/chunk1'
assert_ok depend-1 depend-1.out "$LITAR" -t depend -d 'chunk 1'
assert_ok depend-1b depend-1.out "$LITAR" -t depend -d 'label 1@:chunk 1'
assert_ok depend-2 depend-2.out "$LITAR" -t depend -d 'chunk 2'
assert_ok depend-2b depend-2.out "$LITAR" -t depend -d 'label 2@:chunk 2'
assert_text none '' "$LITAR" -t depend -d nope
