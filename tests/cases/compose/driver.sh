#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text root base "$LITAR" -t . -p msg
assert_text child basechild "$LITAR" -t . -n child -p msg
assert_fail unknown "unknown node 'nope'" "$LITAR" -t . -n nope -p msg
