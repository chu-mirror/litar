#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
exp=$(mktemp)
{
    printf 'usage: %s [options]\n\n' "$LITAR"
    cat <<'EOF'
--help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-t, --target COMPOSITION: specify the composition as COMPOSITION
-n, --node NODE...: specify the node pointed by path NODE...
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
EOF
} >"$exp"
assert_ok help "$exp" "$LITAR" --help
assert_ok help-with-other "$exp" "$LITAR" --help -p
rm -f "$exp"
assert_fail short-h "unknown option '-h'" "$LITAR" -h
assert_fail help-eq "unknown option '--help=foo'" "$LITAR" --help=foo
