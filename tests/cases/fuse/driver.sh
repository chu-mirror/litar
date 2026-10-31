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
assert_text cat $'hello world\n' "$LITAR" -t . -p msg
assert_text exe $'yes\n' "$LITAR" -t . -p ask
assert_text overlay overlay "$LITAR" -t . -p made
assert_fail readonly "exited with status" "$LITAR" -t . -p row
assert_fail missing "file 'missing' is not defined" "$LITAR" -t . -p '@|show@,missing'
assert_fail badpath "invalid file path '../x'" "$LITAR" -t . -p '@|show@,../x'
