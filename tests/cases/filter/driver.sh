#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text upper $'HELLO\n' "$LITAR" -f composer.json -p msg
assert_fail noshebang "filter 'bad' has no shebang" "$LITAR" -f composer.json -p use
assert_fail status "filter 'die' exited with status 3" "$LITAR" -f composer.json -p use2
