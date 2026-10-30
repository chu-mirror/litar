#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text msg $'Hello\n' "$LITAR" --file=composer.json -p msg
assert_text line Hi "$LITAR" -f composer.json -p line
assert_text nest $'!\n\n' "$LITAR" -f composer.json -p base
assert_text attached $'Hello\n' "$LITAR" -f composer.json -pmsg
