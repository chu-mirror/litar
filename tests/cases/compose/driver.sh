#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text root base "$LITAR" -f composer.json -p msg
assert_text child basechild "$LITAR" -f composer.json -n child -p msg
assert_fail unknown "unknown node 'nope'" "$LITAR" -f composer.json -n nope -p msg
