#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
exp=$(mktemp)
cat >"$exp" <<'EOF'
Usage: litar [-a] [-f ARCHIVE] [-l CHUNK_REFERENCE] [-e CHUNK_REFERENCE] [-d CHUNK_REFERENCE] [-r ITEM] [-p EXPRESSION]

-h, --help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-f, --file ARCHIVE: specify the archive file, find the composition and the node corresponding
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
EOF
assert_ok help "$exp" "$LITAR" --help
assert_ok short-h "$exp" "$LITAR" -h
assert_ok help-after-other "$exp" "$LITAR" -p x --help
rm -f "$exp"
assert_fail help-as-value "missing -f, --file" "$LITAR" -p --help
