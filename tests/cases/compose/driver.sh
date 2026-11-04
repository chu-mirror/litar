#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text root base "$LITAR" -f base.la -p msg
assert_text child basechild "$LITAR" -f child.la -p msg
assert_fail unknown "could not find archive 'nope.la'" "$LITAR" -f nope.la -p msg
