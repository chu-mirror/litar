#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_ok literal literal.out "$LITAR" -f literal/literal.la -l 'label@:module@/chunk1'
assert_ok depend-1 depend-1.out "$LITAR" -f depend/depend.la -d 'chunk 1'
assert_ok depend-1b depend-1.out "$LITAR" -f depend/depend.la -d 'label 1@:chunk 1'
assert_ok depend-2 depend-2.out "$LITAR" -f depend/depend.la -d 'chunk 2'
assert_ok depend-2b depend-2.out "$LITAR" -f depend/depend.la -d 'label 2@:chunk 2'
assert_text none '' "$LITAR" -f depend/depend.la -d nope
