#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_text "a dot module names the target module" $'D\n' \
    "$LITAR" -p 'A@/top' archive.la

assert_text "a dot dot module names the parent" $'P\n' \
    "$LITAR" -p 'A@/deep@/sibling' archive.la

assert_text "literal keeps the relative reference" \
    $'@<A@/deep@/sibling@=@<..@/part@>@\n' \
    "$LITAR" -l 'A@/deep@/sibling' archive.la

assert_text "a relative filter is resolved from the target module" $'HI\n' \
    "$LITAR" -p 'A@/deep@/filtered' archive.la

assert_text "the root has two children" $'A\nB\n' \
    "$LITAR" -m / archive.la
assert_text "a module has its immediate child" $'A@/deep\n' \
    "$LITAR" -m A archive.la
assert_text "a leaf has no child" "" "$LITAR" -m 'A@/deep' archive.la

assert_text "a module owns its chunks" $'part\ntop\n' \
    "$LITAR" -c A archive.la
assert_text "a submodule owns its chunks" $'part\nsibling\nfilter\nfiltered\n' \
    "$LITAR" -c 'A@/deep' archive.la
assert_text "the root owns no chunk here" "" "$LITAR" -c / archive.la

assert_text "subdomains of a domain" $'C@/variables\nC@/functions\n' \
    "$LITAR" -s C archive.la
assert_text "a nested domain has no subdomain here" "" \
    "$LITAR" -s 'C@/variables' archive.la
assert_text "items of a domain" $'C@/variables@/x\nC@/variables@/y\n' \
    "$LITAR" -i 'C@/variables' archive.la
assert_text "items of another domain" $'C@/functions@/main\n' \
    "$LITAR" -i 'C@/functions' archive.la

assert_text "reference to one item" \
    $'@<A@/part@=\nP\n@~C@/variables@/y@~C@/functions@/main@\n' \
    "$LITAR" -r 'C@/variables@/y' archive.la
