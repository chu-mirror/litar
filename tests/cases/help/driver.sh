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
-t, --target COMPOSER_DIRECTORY: read in the composer file in COMPOSER_DIRECTORY
-n, --node NODE: specify the NODE
-m, --modules MODULE: list all modules under MODULE
-c, --chunks MODULE: list all chunk names under MODULE
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-s, --subdomains DOMAIN: list all subdomains under DOMAIN
-i, --items DOMAIN: list all items under DOMAIN
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
EOF
} >"$exp"
assert_ok help "$exp" "$LITAR" --help
assert_ok help-with-other "$exp" "$LITAR" --help -p
rm -f "$exp"
assert_fail short-h "unknown option '-h'" "$LITAR" -h
assert_fail help-eq "unknown option '--help=foo'" "$LITAR" --help=foo
