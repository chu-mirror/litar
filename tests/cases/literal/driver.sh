#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_ok "design example" outline.out \
    "$LITAR" -l 'label@:module 1@/chunk' outline.la
assert_text "other chunk is left out of the example" \
    $'@<module 1@/chunk2@=\n@<substructure@>\n@\n' \
    "$LITAR" -l chunk2 outline.la
assert_text "no module still names the module" \
    $'@<module 1@/chunk@=\n@<substructure 1@>\n@\n' \
    "$LITAR" -l chunk outline.la
assert_text "named module keeps that module" \
    $'@<module 1@/chunk@=\n@<substructure 1@>\n@\n' \
    "$LITAR" -l 'module 1@/chunk' outline.la
assert_fail "other module is a different chunk" \
    "chunk 'label@:module 2@/chunk' is not defined" \
    "$LITAR" -l 'label@:module 2@/chunk' outline.la
assert_fail "unspecialized name misses the labeled block" \
    "chunk 'module 2@/chunk' is not defined" \
    "$LITAR" -l 'module 2@/chunk' outline.la

assert_ok "each block names its module" modinfo.out \
    "$LITAR" -l chunk modinfo.la
assert_text "one module names that module on each block" \
    $'@<One@/chunk@=\nA\n@\n\n@<One@/chunk@=\nB\n@\n' \
    "$LITAR" -l 'One@/chunk' modinfo.la
assert_text "header names another module" \
    $'@<Two@/chunk@=\nC\n@\n' \
    "$LITAR" -l 'Two@/chunk' modinfo.la
assert_ok "anonymous file extends another module" cross.out \
    "$LITAR" -l 'Named@/includes' cross.la

assert_text "literal keeps the block text" \
    $'@<chunk@=\none\n@<other@>\n@\n\n@<chunk@=\n@@\n@\n' \
    "$LITAR" -l chunk archive.la
assert_text "print expands references" $'one\nTWO\n\n@\n' \
    "$LITAR" -p chunk archive.la
assert_text "literal keeps @@" $'@<escaped@=\n@@<not a ref@@>\n@\n' \
    "$LITAR" -l escaped archive.la
assert_text "literal keeps paired @@" $'@<at@=\n@@@@\n@\n' \
    "$LITAR" -l at archive.la
assert_text "literal keeps a filter" $'@<filtered@=\nhello\n@|upper@\n' \
    "$LITAR" -l filtered archive.la
assert_text "print runs a block filter" $'HELLO\n' \
    "$LITAR" -p filtered archive.la
assert_text "literal keeps a missing filter" \
    $'@<nofilter@=\nraw\n@|missing-filter@\n' \
    "$LITAR" -l nofilter archive.la
assert_fail "print requires the filter" "filter 'missing-filter' is not defined" \
    "$LITAR" -p nofilter archive.la

assert_text "unspecialized only" $'@<base@=\nB\n@\n' \
    "$LITAR" -l base archive.la
assert_text "label and predecessor" \
    $'@<lab@:base@=\nB\n@\n\n@<lab@:base@=\nL\n@\n' \
    "$LITAR" -l 'lab@:base' archive.la
assert_text "other label" \
    $'@<otherlab@:base@=\nB\n@\n\n@<otherlab@:base@=\nO\n@\n' \
    "$LITAR" -l 'otherlab@:base' archive.la
assert_text "deeper specialization" \
    $'@<lab@:extra@:base@=\nB\n@\n\n@<lab@:extra@:base@=\nX\n@\n' \
    "$LITAR" -l 'lab@:extra@:base' archive.la
assert_text "label group" \
    $'@<lab@:extra@:base@=\nB\n@\n\n@<lab@:extra@:base@=\nX\n@\n' \
    "$LITAR" --literal 'pair@:base' archive.la

assert_text "predecessor receives the requested labels" \
    $'@<label@:outline@=\n@<label@:substructure 1@>\n@\n\n@<label@:outline@=\n@<substructure 2@>\n@\n' \
    "$LITAR" -l 'label@:outline' archive.la
assert_text "explicit labels stay, and the predecessor is included" \
    $'@<kept@:outline@=\n@<kept@:substructure 1@>\n@\n\n@<kept@:outline@=\n@<keep@:named@>\n@<open@>\n@\n' \
    "$LITAR" -l 'kept@:outline' archive.la
assert_text "less specialized bodies receive every requested label" \
    $'@<a@:b@:outline@=\n@<a@:b@:substructure 1@>\n@\n\n@<a@:b@:outline@=\n@<a@:b@:inner@>\n@\n\n@<a@:b@:outline@=\n@<inner@>\n@<Mod@/inner@>\n@<inner@|upper@>\n@\n' \
    "$LITAR" -l 'a@:b@:outline' archive.la
assert_text "label group is passed into the body" \
    $'@<lab@:extra@:grouped@=\n@<lab@:extra@:hole@>\n@\n' \
    "$LITAR" -l 'pair@:grouped' archive.la

assert_text "literal does not follow a cycle" $'@<cycle@=\n@<cycle@>\n@\n' \
    "$LITAR" -l cycle archive.la
assert_fail "print rejects a cycle" "circular inclusion of 'cycle'" \
    "$LITAR" -p cycle archive.la
assert_text "skipped branch is not a block" $'@<branched@=\nyes\n@\n' \
    "$LITAR" -l branched archive.la

user_meta=$'@<Mod@/meta@=\n{\n    summary: |||\n        @<summary@>\n    |||,\n}\n@\n'
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
assert_text "specialization passes labels into an unlabeled reference" \
    $'@<label@:Mod@/meta@=\n{\n    summary: |||\n        @<label@:summary@>\n    |||,\n}\n@\n' \
    "$LITAR" -l 'label@:Mod@/meta' archive.la
assert_fail "unknown module has no meta" "chunk 'NoSuch@/meta' is not defined" \
    "$LITAR" -l 'NoSuch@/meta' archive.la

assert_text "included block stays literal" $'@<fromlib@=\nlib @<chunk@>\n@\n' \
    "$LITAR" --literal=fromlib archive.la
assert_text "included predecessor receives labels" \
    $'@<lab@:fromlib@=\nlib @<lab@:chunk@>\n@\n' \
    "$LITAR" -l 'lab@:fromlib' archive.la
assert_fail "undefined chunk" "chunk 'missing' is not defined" \
    "$LITAR" -l missing archive.la
assert_fail "filters are not part of a chunk reference" \
    "unexpected control '@|' in name" \
    "$LITAR" -l 'chunk@|upper' archive.la
assert_text "names write @ as @@" \
    $'@<a@@b@/a@@c@=\nz\n@\n' \
    "$LITAR" -l 'a@@b@/a@@c' archive.la
assert_text "a passed label writes @ as @@" \
    $'@<m@@n@:a@@b@/a@@c@=\nz\n@\n' \
    "$LITAR" -l 'm@@n@:a@@b@/a@@c' archive.la

carrier=$'@<mark@:mid@:carrier@=\n@@<not@@>\n@<mark@:mid@:real@>\n@<keep@:named@>\n@<mark@:mid@:Mod@/inner@>\n@<mark@:mid@:inner@|upper@>\n@|upper@\n\n@<mark@:mid@:carrier@=\n@<mark@:mid@:hole@>\n@\n\n@<mark@:mid@:carrier@=\nkept\n@\n'
assert_text "labels pass through references and stop at the block filter" \
    "$carrier" "$LITAR" -l 'mark@:mid@:carrier' archive.la
assert_text "an unspecialized request does not pass labels" \
    $'@<carrier@=\n@@<not@@>\n@<real@>\n@<keep@:named@>\n@<Mod@/inner@>\n@<inner@|upper@>\n@|upper@\n' \
    "$LITAR" -l carrier archive.la
