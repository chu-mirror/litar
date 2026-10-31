#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text msg $'Hello\n' "$LITAR" --target=. -p msg
assert_text line Hi "$LITAR" -t . -p line
assert_text nest $'!\n\n' "$LITAR" -t . -p base
assert_text attached $'Hello\n' "$LITAR" -t. -pmsg
