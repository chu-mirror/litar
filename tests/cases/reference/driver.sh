#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

times=$'Variable \'times\' saves the number of occurrences of an event.\n@<chunk@=\nint times;\n@|filter\n@~ C@/variables@/times @\n@<shown@=\nint times;\n@~C@/variables@/times@\n@<shown@+\nmore\n@~C@/variables@/times@/extra@\n@<both@=\ntwo\n@~C@/variables@/times@~C@/functions@/main@\n@<plain@=\nint times;\n@~ C @/ variables @/ times @\n'
variables="${times}"$'@<fromlib@=\nlib\n@~C@/variables@/fromlib@\n\n@<elsechunk@=\nyes\n@~C@/variables@/else@\n@<lab@:Mod@/kept@=\nstill\n@~C@/variables@/n@\n'
domain=$'Variable \'times\' saves the number of occurrences of an event.\n@<chunk@=\nint times;\n@|filter\n@~ C@/variables@/times @\n@<shown@=\nint times;\n@~C@/variables@/times@\n@<shown@+\nmore\n@~C@/variables@/times@/extra@\n@<broad@=\nonly\n@~C@\n@<other@=\nno\n@~C@/functions@/main@\n@<both@=\ntwo\n@~C@/variables@/times@~C@/functions@/main@\n@<plain@=\nint times;\n@~ C @/ variables @/ times @\n@<fromlib@=\nlib\n@~C@/variables@/fromlib@\n\n@<elsechunk@=\nyes\n@~C@/variables@/else@\n@<lab@:Mod@/kept@=\nstill\n@~C@/variables@/n@\n'

assert_text "further names still match" "$times" \
    "$LITAR" -r 'C@/variables@/times' archive.la
assert_text "spaces in the query are trimmed" "$variables" \
    "$LITAR" -r ' C @/ variables ' archive.la
assert_text "one more classification" \
    $'@<shown@+\nmore\n@~C@/variables@/times@/extra@\n' \
    "$LITAR" -r 'C@/variables@/times@/extra' archive.la
assert_text "a domain matches every classification" "$domain" \
    "$LITAR" -r C archive.la
assert_text "--reference=" "$domain" \
    "$LITAR" --reference=C archive.la
assert_text "archive before -r" \
    $'@<fromlib@=\nlib\n@~C@/variables@/fromlib@\n' \
    "$LITAR" archive.la -r 'C@/variables@/fromlib'
assert_text "a branch of the domain" \
    $'@<other@=\nno\n@~C@/functions@/main@\n@<both@=\ntwo\n@~C@/variables@/times@~C@/functions@/main@\n' \
    "$LITAR" -r 'C@/functions' archive.la
assert_text "variables keeps both topics on one block" "$variables" \
    "$LITAR" -r 'C@/variables' archive.la
assert_text "@@ in a topic name" \
    $'@<Mod@/kept@=\nmod\n@~a@@b@/item@\n' \
    "$LITAR" -r 'a@@b' archive.la
assert_text "@@ and a further name" \
    $'@<Mod@/kept@=\nmod\n@~a@@b@/item@\n' \
    "$LITAR" -r 'a@@b@/item' archive.la
assert_text "labels do not select the topic" \
    $'@<lab@:Mod@/kept@=\nstill\n@~C@/variables@/n@\n' \
    "$LITAR" -r 'C@/variables@/n' archive.la
assert_text "--reference" \
    $'@<elsechunk@=\nyes\n@~C@/variables@/else@\n' \
    "$LITAR" --reference 'C@/variables@/else' archive.la
assert_text "an included block" \
    $'@<fromlib@=\nlib\n@~C@/variables@/fromlib@\n' \
    "$LITAR" -r 'C@/variables@/fromlib' archive.la

assert_text "a shorter name is a different topic" "" \
    "$LITAR" -r 'C@/var' archive.la
assert_text "a middle name is not a domain" "" \
    "$LITAR" -r variables archive.la
assert_text "a skipped arm is not recorded" "" \
    "$LITAR" -r 'C@/variables@/hidden' archive.la
assert_text "a topic is not a chunk use" "" \
    "$LITAR" -d times archive.la

assert_text "evaluation omits the topic" $'int times;\nmore\n' \
    "$LITAR" -p shown archive.la
assert_text "@@~ stays in the content" $'@~not a topic\n' \
    "$LITAR" -p 'literal at' archive.la
assert_text "literal keeps the topic text" \
    $'@<shown@=\nint times;\n@~C@/variables@/times@\n@<shown@+\nmore\n@~C@/variables@/times@/extra@\n' \
    "$LITAR" -l shown archive.la
assert_text "an escaped tilde is not a topic" \
    $'@<literal at@=\n@@~not a topic\n@\n' \
    "$LITAR" -l 'literal at' archive.la

assert_fail "skipped chunk is not defined" "chunk 'hidden' is not defined" \
    "$LITAR" -l hidden archive.la

agent_main=$'[\n    {\n        "comment": "",\n        "labels": [],\n        "module": "",\n        "chunk": "other",\n        "type": "set",\n        "content": "no\\n",\n        "filters": [],\n        "topics": [\n            ["C", "functions", "main"]\n        ],\n        "file": "archive.la",\n        "line-range": [15, 17]\n    },\n    {\n        "comment": "",\n        "labels": [],\n        "module": "",\n        "chunk": "both",\n        "type": "set",\n        "content": "two\\n",\n        "filters": [],\n        "topics": [\n            ["C", "variables", "times"],\n            ["C", "functions", "main"]\n        ],\n        "file": "archive.la",\n        "line-range": [18, 20]\n    }\n]\n'
assert_text "agent listing" "$agent_main" \
    "$LITAR" -a -r 'C@/functions@/main' archive.la

agent_kept=$'[\n    {\n        "comment": "",\n        "labels": [],\n        "module": "Mod",\n        "chunk": "kept",\n        "type": "set",\n        "content": "mod\\n",\n        "filters": [],\n        "topics": [\n            ["a@b", "item"]\n        ],\n        "file": "archive.la",\n        "line-range": [37, 39]\n    }\n]\n'
assert_text "agent decodes @@" "$agent_kept" \
    "$LITAR" -a -r 'a@@b' archive.la

agent_lib=$'[\n    {\n        "comment": "",\n        "labels": [],\n        "module": "",\n        "chunk": "fromlib",\n        "type": "set",\n        "content": "lib\\n",\n        "filters": [],\n        "topics": [\n            ["C", "variables", "fromlib"]\n        ],\n        "file": "./lib.la",\n        "line-range": [1, 3]\n    }\n]\n'
assert_text "agent names the included file" "$agent_lib" \
    "$LITAR" -a -r 'C@/variables@/fromlib' archive.la
assert_text "agent listing of an unused topic" $'[]\n' \
    "$LITAR" -a -r variables archive.la

bad=$(mktemp)
printf '%s' $'@<c@=\nbody\n@~@\n' >"$bad"
assert_fail "empty topic name" "empty topic name" "$LITAR" -l c "$bad"
printf '%s' $'@<c@=\nbody\n@~C@/@\n' >"$bad"
assert_fail "empty topic segment" "empty topic name" "$LITAR" -l c "$bad"
printf '%s' $'@<c@=\nbody\n@~C@:x@\n' >"$bad"
assert_fail "unexpected control in a topic" "unexpected control '@:' in name" \
    "$LITAR" -l c "$bad"
printf '%s' $'@~C@\n' >"$bad"
assert_fail "top-level topic" "unknown expression '@~'" "$LITAR" -l c "$bad"
printf '%s' $'@<c@=\nbody\n@~C@/x' >"$bad"
assert_fail "unterminated topic" "unterminated expression" "$LITAR" -l c "$bad"
printf '%s' $'@<c@=\nbody\n@~C@|filt@\n' >"$bad"
assert_fail "a filter is not a topic name" "unexpected control '@|' in name" \
    "$LITAR" -l c "$bad"
printf '%s' $'@<c@=\nbody\n@~C@\n@|filt@\n' >"$bad"
assert_fail "a pipeline after the block is a new expression" "unexpected '@|'" \
    "$LITAR" -l c "$bad"
printf '%s' $'@? no @|\n@<h@=\nx\n@~@\n@|\n@<e@=\ny\n@\n' >"$bad"
assert_fail "a skipped arm still parses the topic" "empty topic name" \
    "$LITAR" -l e "$bad"
printf '%s' $'@<inner@=\nsee @<name@~still@>\n@\n' >"$bad"
assert_text "a tilde inside a reference stays in the body" \
    $'@<inner@=\nsee @<name@~still@>\n@\n' \
    "$LITAR" -l inner "$bad"
assert_text "a tilde inside a reference is not a topic" "" \
    "$LITAR" -r still "$bad"
assert_fail "evaluation still parses the reference" \
    "unexpected control '@~' in name" \
    "$LITAR" -p inner "$bad"
rm -f "$bad"

assert_fail "unexpected control in the query" "unexpected control '@:' in name" \
    "$LITAR" -r 'C@:' archive.la
assert_fail "empty query segment" "empty topic name" \
    "$LITAR" -r 'C@/' archive.la
assert_fail "a bare @ is an empty name" "empty topic name" \
    "$LITAR" -r '@' archive.la
assert_fail "a query does not take a closing @" "unterminated name" \
    "$LITAR" -r 'C@/variables@' archive.la
assert_fail "a filter mark in the query" "unexpected control '@|' in name" \
    "$LITAR" --reference 'C@|x' archive.la
