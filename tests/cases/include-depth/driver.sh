#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"

gen_chain() {
    dir=$1
    count=$2
    mkdir -p "$dir"
    i=1
    while [ "$i" -lt "$count" ]; do
        next=$((i + 1))
        printf '@. f%d.la @\n' "$next" >"$dir/f$i.la"
        i=$next
    done
    printf '@<ok@=\nYES\n@\n' >"$dir/f$count.la"
    printf '@. f1.la @\n' >"$dir/main.la"
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
gen_chain "$tmp/ok" 63
(
    cd "$tmp/ok"
    assert_text "64 archives" $'YES\n' "$LITAR" -p ok main.la
)
gen_chain "$tmp/bad" 64
(
    cd "$tmp/bad"
    assert_fail "65 archives" "includes are nested too deeply in './f64.la'" \
        "$LITAR" -p ok main.la
)
