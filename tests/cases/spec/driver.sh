#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text plain A "$LITAR" -f composer.json -p c
assert_text mid AB "$LITAR" -f composer.json -p 's1@:c'
assert_text full Z "$LITAR" -f composer.json -p 's2@:s1@:c'
assert_text other AD "$LITAR" -f composer.json -p 's2@:c'
assert_text effective $'@<s2@:s1@:c@=Z@\n' "$LITAR" -f composer.json -e 's2@:s1@:c'
assert_text literal $'@<c@+A@\n@<s1@:c@+B@\n@<s2@:s1@:c@+C@\n@<s2@:s1@:c@=Z@\n' \
    "$LITAR" -f composer.json -l 's2@:s1@:c'
