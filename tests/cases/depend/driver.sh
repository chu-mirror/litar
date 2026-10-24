#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

assert_ok "users of hole" hole.out "$LITAR" -d hole archive.la
assert_ok "long option" hole.out "$LITAR" --depend hole archive.la
assert_ok "--depend=" hole.out "$LITAR" --depend=hole archive.la
assert_ok "archive before -d" hole.out "$LITAR" archive.la -d hole

assert_ok "module limits the use" mod-hole.out \
    "$LITAR" -d 'Mod@/hole' archive.la
assert_text "other module is a different use" "" \
    "$LITAR" -d 'Reader@/hole' archive.la

assert_ok "a specialized filter depends on the base" upper.out \
    "$LITAR" -d 'Mod@/upper' archive.la
assert_ok "the reference itself" user.out \
    "$LITAR" -d 'Mod@/other' archive.la
assert_ok "a module limits a specialized filter" upper.out \
    "$LITAR" -d 'Mod@/upper' archive.la

assert_ok "labels inherited from the block" lab-hole.out \
    "$LITAR" -d 'lab@:Mod@/hole' archive.la
assert_text "a root query does not match a module use" "" \
    "$LITAR" -d 'lab@:hole' archive.la

assert_ok "block filter inherits the block labels" lab-upper.out \
    "$LITAR" -d 'lab@:Mod@/upper' archive.la
assert_text "an explicit filter does not inherit" "" \
    "$LITAR" -d 'plain@:upper' archive.la

assert_ok "filter labels are exact" marked.out \
    "$LITAR" -d 'red@:Mod@/upper' archive.la
assert_ok "filter module is the defining module" marked.out \
    "$LITAR" -d 'red@:Mod@/upper' archive.la
assert_ok "the filtered reference" marked.out \
    "$LITAR" -d text archive.la
assert_text "explicit filter labels do not match a longer name" "" \
    "$LITAR" -d 'red@:blue@:Mod@/upper' archive.la
assert_text "explicit reference labels do not grow" "" \
    "$LITAR" -d 'extra@:lab@:Mod@/hole' archive.la

assert_ok "label group on the block and on the use" grouped.out \
    "$LITAR" -d 'pair@:Mod@/hole' archive.la
assert_ok "expanded group" grouped.out \
    "$LITAR" -d 'red@:blue@:Mod@/hole' archive.la
assert_ok "group on the query expands before the module is applied" grouped.out \
    "$LITAR" -d 'pair@:Mod@/hole' archive.la
assert_ok "a suffix label is the same specialization path" grouped.out \
    "$LITAR" -d 'blue@:Mod@/hole' archive.la
assert_text "a different specialization path does not match" "" \
    "$LITAR" -d 'red@:Mod@/hole' archive.la

assert_ok "a block that names itself" cycle.out \
    "$LITAR" -d 'Other@/cycle' archive.la
assert_text "undefined and unused" "" "$LITAR" -d missing archive.la
assert_text "extending a chunk is not depending on it" "" \
    "$LITAR" -d 'Mod@/user' archive.la
assert_text "skipped arm is not a block" "" \
    "$LITAR" -d 'Mod@/hidden' archive.la

assert_ok "@@ in the module and the chunk" kept.out \
    "$LITAR" -d 'a@@b@/kept' archive.la
assert_text "@@ in a comment is not a reference" "" \
    "$LITAR" -d anchor archive.la
assert_text "print still expands @@" $'@<hole@>\n' \
    "$LITAR" -p anchor archive.la

assert_fail "filters are not part of a chunk reference" \
    "unexpected control '@|' in name" \
    "$LITAR" -d 'user@|upper' archive.la
assert_fail "empty reference" "empty chunk name" \
    "$LITAR" --depend= archive.la

assert_ok "design example, unspecialized use" outline-sub1.out \
    "$LITAR" -d 'module 1@/substructure 1' outline.la
assert_text "a root query does not match the use" "" \
    "$LITAR" -d 'substructure 1' outline.la
assert_text "outer labels are not applied" "" \
    "$LITAR" -d 'label@:substructure 1' outline.la
assert_text "extending chunk is not a use of chunk" "" \
    "$LITAR" -d 'module 1@/chunk' outline.la

assert_ok "inherited label and target module" outline-sub2.out \
    "$LITAR" -d 'label@:module 1@/substructure 2' outline.la
assert_text "a root query does not match the module use" "" \
    "$LITAR" -d 'label@:substructure 2' outline.la
assert_text "a root query does not match the unspecialized use" "" \
    "$LITAR" -d 'substructure 2' outline.la
assert_ok "the target module names the use" outline-sub2.out \
    "$LITAR" -d 'module 1@/substructure 2' outline.la

assert_ok "another unspecialized use" outline-sub.out \
    "$LITAR" -d 'module 1@/substructure' outline.la

assert_ok "design example contains a specialized reference" example-chunk1.out \
    "$LITAR" -d 'chunk 1' example.la
assert_ok "design example names the specialized reference" example-chunk1.out \
    "$LITAR" -d 'label 1@:chunk 1' example.la
assert_text "a longer specialization is a different reference" "" \
    "$LITAR" -d 'extra@:label 1@:chunk 1' example.la
assert_ok "design example is filtered by a specialized chunk" example-chunk2.out \
    "$LITAR" -d 'chunk 2' example.la
assert_ok "design example names the filter" example-chunk2.out \
    "$LITAR" -d 'label 2@:chunk 2' example.la
assert_text "another label is another path" "" \
    "$LITAR" -d 'label 1@:chunk 2' example.la
