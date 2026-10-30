#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
if [ ! -e /dev/fuse ]; then
    echo "skip fuse: no /dev/fuse"
    exit 0
fi
cd "$(dirname "$0")"
. "$ROOT/tests/assert.sh"
assert_text cat $'hello world\n' "$LITAR" -f composer.json -p msg
assert_text exe $'yes\n' "$LITAR" -f composer.json -p ask
assert_text overlay overlay "$LITAR" -f composer.json -p made
assert_fail readonly "exited with status" "$LITAR" -f composer.json -p row
assert_fail missing "file 'missing' is not defined" "$LITAR" -f composer.json -p '@|show@,missing'
assert_fail badpath "invalid file path '../x'" "$LITAR" -f composer.json -p '@|show@,../x'
