#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_ok "design example" outline.out \
    "$LITAR" -l 'label@:chunk' outline.la
assert_text "other chunk is left out of the example" \
    $'@- module 1 @\n\n@<chunk2@=\n@<substructure@>\n@\n' \
    "$LITAR" -l chunk2 outline.la
assert_text "no module still shows the selecting module" \
    $'@- module 1 @\n\n@<chunk@=\n@<substructure 1@>\n@\n' \
    "$LITAR" -l chunk outline.la
assert_text "named module keeps that module" \
    $'@- module 1 @\n\n@<chunk@=\n@<substructure 1@>\n@\n' \
    "$LITAR" -l 'module 1@/chunk' outline.la
assert_text "other module is a different chunk" \
    $'\n@- module 2 @\n\nExtend chunk through label.\n@<label@:chunk@=\n@<substructure 2@>\n@\n' \
    "$LITAR" -l 'label@:module 2@/chunk' outline.la
assert_fail "unspecialized name misses the labeled block" \
    "chunk 'module 2@/chunk' is not defined" \
    "$LITAR" -l 'module 2@/chunk' outline.la

assert_ok "module selections are attached" modinfo.out \
    "$LITAR" -l chunk modinfo.la
assert_text "one module prints its @- once" \
    $'@- One @\n\n@<chunk@=\nA\n@\n\n@<chunk@=\nB\n@\n' \
    "$LITAR" -l 'One@/chunk' modinfo.la
assert_text "header names another module, extending stays in One" \
    $'@- One @\n\n@<Two@/chunk@=\nC\n@\n' \
    "$LITAR" -l 'Two@/chunk' modinfo.la
assert_ok "anonymous file extends another module" cross.out \
    "$LITAR" -l 'Named@/includes' cross.la

assert_text "literal keeps the block text" \
    $'@-@\n@<chunk@=\none\n@<other@>\n@\n\n@<chunk@=\n@@\n@\n' \
    "$LITAR" -l chunk archive.la
assert_text "print expands references" $'one\nTWO\n\n@\n' \
    "$LITAR" -p chunk archive.la
assert_text "literal keeps @@" $'@-@\n\n@<escaped@=\n@@<not a ref@@>\n@\n' \
    "$LITAR" -l escaped archive.la
assert_text "literal keeps paired @@" $'@-@\n\n@<at@=\n@@@@\n@\n' \
    "$LITAR" -l at archive.la
assert_text "literal keeps a filter" $'@-@\n\n@<filtered@=\nhello\n@|upper@\n' \
    "$LITAR" -l filtered archive.la
assert_text "print runs a block filter" $'HELLO\n' \
    "$LITAR" -p filtered archive.la
assert_text "literal keeps a missing filter" \
    $'@-@\n\n@<nofilter@=\nraw\n@|missing-filter@\n' \
    "$LITAR" -l nofilter archive.la
assert_fail "print requires the filter" "filter 'missing-filter' is not defined" \
    "$LITAR" -p nofilter archive.la

assert_text "unspecialized only" $'@-@\n\n@<base@=\nB\n@\n' \
    "$LITAR" -l base archive.la
assert_text "label and predecessor" \
    $'@-@\n\n@<base@=\nB\n@\n\n@<lab@:base@=\nL\n@\n' \
    "$LITAR" -l 'lab@:base' archive.la
assert_text "other label" \
    $'@-@\n\n@<base@=\nB\n@\n\n@<otherlab@:base@=\nO\n@\n' \
    "$LITAR" -l 'otherlab@:base' archive.la
assert_text "deeper specialization" \
    $'@-@\n\n@<base@=\nB\n@\n\n@<lab@:base@=\nL\n@\n\n@<lab@:extra@:base@=\nX\n@\n' \
    "$LITAR" -l 'lab@:extra@:base' archive.la
assert_text "label group" \
    $'@-@\n\n@<base@=\nB\n@\n\n@<lab@:base@=\nL\n@\n\n@<lab@:extra@:base@=\nX\n@\n' \
    "$LITAR" --literal 'pair@:base' archive.la

assert_text "predecessor block is printed whole" \
    $'@-@\n\n@<outline@=\n@<substructure 1@>\n@\n\n@<label@:outline@=\n@<substructure 2@>\n@\n' \
    "$LITAR" -l 'label@:outline' archive.la
assert_text "explicit labels stay, and the predecessor is included" \
    $'@-@\n\n@<outline@=\n@<substructure 1@>\n@\n\n@<kept@:outline@=\n@<keep@:named@>\n@<open@>\n@\n' \
    "$LITAR" -l 'kept@:outline' archive.la
assert_text "matching specializations, references unchanged" \
    $'@-@\n\n@<outline@=\n@<substructure 1@>\n@\n\n@<b@:outline@=\n@<inner@>\n@\n\n@<a@:b@:outline@=\n@<inner@>\n@<Mod@/inner@>\n@<inner@|upper@>\n@\n' \
    "$LITAR" -l 'a@:b@:outline' archive.la
assert_text "label group does not rewrite the body" \
    $'@-@\n\n@<grouped@=\n@<hole@>\n@\n' \
    "$LITAR" -l 'pair@:grouped' archive.la

assert_text "literal does not follow a cycle" $'@-@\n\n@<cycle@=\n@<cycle@>\n@\n' \
    "$LITAR" -l cycle archive.la
assert_fail "print rejects a cycle" "circular inclusion of 'cycle'" \
    "$LITAR" -p cycle archive.la
assert_text "skipped branch is not a block" $'@-@\n\n\n@<branched@=\nyes\n@\n' \
    "$LITAR" -l branched archive.la

user_meta=$'\n@- Mod @\n\n@<meta@=\n{\n    summary: |||\n        @<summary@>\n    |||,\n}\n@\n'
user_exp=$'{\n    summary: |||\n        hello module\n\n    |||,\n}\n'
assert_fail "print keeps the anonymous module" "chunk 'meta' is not defined" \
    "$LITAR" -p meta archive.la
assert_text "literal with no module finds Mod" "$user_meta" \
    "$LITAR" -l meta archive.la
assert_text "author meta is an ordinary block" "$user_meta" \
    "$LITAR" -l 'Mod@/meta' archive.la
assert_text "author meta evaluates" "$user_exp" \
    "$LITAR" -p 'Mod@/meta' archive.la
assert_fail "selected module has no built-in meta" "chunk 'Ghost@/meta' is not defined" \
    "$LITAR" -l 'Ghost@/meta' archive.la
assert_text "specialization does not rewrite references" "$user_meta" \
    "$LITAR" -l 'label@:Mod@/meta' archive.la
assert_fail "unknown module has no meta" "chunk 'NoSuch@/meta' is not defined" \
    "$LITAR" -l 'NoSuch@/meta' archive.la

assert_text "included block stays literal" $'@-@\n@<fromlib@=\nlib @<chunk@>\n@\n' \
    "$LITAR" --literal=fromlib archive.la
assert_fail "undefined chunk" "chunk 'missing' is not defined" \
    "$LITAR" -l missing archive.la
assert_fail "filters are not part of a chunk reference" \
    "unexpected control '@|' in name" \
    "$LITAR" -l 'chunk@|upper' archive.la
