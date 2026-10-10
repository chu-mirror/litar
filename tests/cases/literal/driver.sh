#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_text "literal keeps references" $'one\n@<other@>\n@\n' \
    "$LITAR" -l chunk archive.la
assert_text "print expands references" $'one\nTWO\n\n@\n' \
    "$LITAR" -p chunk archive.la
assert_text "literal decodes @@" $'@<not a ref@>\n' \
    "$LITAR" -l escaped archive.la
assert_text "literal decodes paired @@" $'@@\n' \
    "$LITAR" -l at archive.la
assert_text "literal skips a working filter" $'hello\n' \
    "$LITAR" -l filtered archive.la
assert_text "print runs a block filter" $'HELLO\n' \
    "$LITAR" -p filtered archive.la
assert_text "literal skips a missing filter" $'raw\n' \
    "$LITAR" -l nofilter archive.la
assert_fail "print requires the filter" "filter 'missing-filter' is not defined" \
    "$LITAR" -p nofilter archive.la

assert_text "unspecialized only" $'B\n' \
    "$LITAR" -l base archive.la
assert_text "label and predecessor" $'B\nL\n' \
    "$LITAR" -l 'lab@:base' archive.la
assert_text "other label" $'B\nO\n' \
    "$LITAR" -l 'otherlab@:base' archive.la
assert_text "deeper specialization" $'B\nL\nX\n' \
    "$LITAR" -l 'lab@:extra@:base' archive.la
assert_text "label group" $'B\nL\nX\n' \
    "$LITAR" --literal 'pair@:base' archive.la

assert_text "pass label into the unspecialized block" \
    $'@<label@:substructure 1@>\n@<substructure 2@>\n' \
    "$LITAR" -l 'label@:outline' archive.la
assert_text "explicit labels stay" \
    $'@<kept@:substructure 1@>\n@<keep@:named@>\n@<open@>\n' \
    "$LITAR" -l 'kept@:outline' archive.la
assert_text "only labels the block does not carry" \
    $'@<a@:b@:substructure 1@>\n@<a@:inner@>\n@<inner@>\n@<Mod@/inner@>\n@<inner@|upper@>\n' \
    "$LITAR" -l 'a@:b@:outline' archive.la
assert_text "label group is expanded into the reference" \
    $'@<lab@:extra@:hole@>\n' \
    "$LITAR" -l 'pair@:grouped' archive.la

assert_text "literal does not follow a cycle" $'@<cycle@>\n' \
    "$LITAR" -l cycle archive.la
assert_fail "print rejects a cycle" "circular inclusion of 'cycle'" \
    "$LITAR" -p cycle archive.la
assert_text "skipped branch is not a block" $'yes\n' \
    "$LITAR" -l branched archive.la

meta_src=$'{\n    summary: |||\n        @<summary@>\n    |||,\n    exported: [@<exported chunks@>],\n}\n'
user_meta=$'{\n    summary: |||\n        @<summary@>\n    |||,\n}\n'
anon_exp=$'{\n    summary: |||\n        \n    |||,\n    exported: [],\n}\n'
mod_exp=$'{\n    summary: |||\n        hello module\n\n    |||,\n    exported: [],\n}\n'
user_exp=$'{\n    summary: |||\n        hello module\n\n    |||,\n}\n'
assert_text "anonymous meta is built in" "$meta_src" \
    "$LITAR" -l meta archive.la
assert_text "anonymous meta evaluates holes" "$anon_exp" \
    "$LITAR" -p meta archive.la
assert_text "module meta keeps author blocks" "$meta_src$user_meta" \
    "$LITAR" -l 'Mod@/meta' archive.la
assert_text "module meta evaluates" "$mod_exp$user_exp" \
    "$LITAR" -p 'Mod@/meta' archive.la
assert_text "selected module has meta" "$meta_src" \
    "$LITAR" -l 'Ghost@/meta' archive.la
assert_text "labels pass into built-in meta" \
    $'{\n    summary: |||\n        @<label@:summary@>\n    |||,\n    exported: [@<label@:exported chunks@>],\n}\n' \
    "$LITAR" -l 'label@:meta' archive.la
assert_fail "unknown module has no meta" "chunk 'NoSuch@/meta' is not defined" \
    "$LITAR" -l 'NoSuch@/meta' archive.la

assert_text "included block stays literal" $'lib @<chunk@>\n' \
    "$LITAR" --literal=fromlib archive.la
assert_fail "undefined chunk" "chunk 'missing' is not defined" \
    "$LITAR" -l missing archive.la
assert_fail "filters are not part of a chunk reference" \
    "unexpected control '@|' in name" \
    "$LITAR" -l 'chunk@|upper' archive.la
