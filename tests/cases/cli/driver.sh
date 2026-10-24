#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"
cd "$(dirname "$0")"

check_help() {
    _name=$1
    shift
    _out=$(mktemp)
    _err=$(mktemp)
    set +e
    "$@" >"$_out" 2>"$_err"
    _st=$?
    set -e
    if [ "$_st" -ne 0 ]; then
        echo "FAIL $_name: exit $_st" >&2
        cat "$_err" >&2
        exit 1
    fi
    if [ -s "$_err" ]; then
        echo "FAIL $_name: stderr" >&2
        cat "$_err" >&2
        exit 1
    fi
    for _needle in "usage: $LITAR [options] ARCHIVE" \
        "--help: print the usage" \
        "-p, --print EXPRESSION: evaluate EXPRESSION, and print it" \
        "-m, --modules MODULE: list all modules under MODULE" \
        "-c, --chunks MODULE: list all chunks under MODULE" \
        "-s, --subdomains DOMAIN: list all subdomains under DOMAIN" \
        "-i, --item DOMAIN: list all items under DOMAIN" \
        "-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally" \
        "-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE" \
        "-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE" \
        "-r, --reference ITEM: list all blocks that refer to ITEM" \
        "-a, --agent: alter output to suit an agent's needs" \
        "MODULE follows module@/submodule@/subsubmodule; the root module is /." \
        "label1@:label2@:module name@/chunk name@|filter1@|filter2" \
        "label1@:label2@:module name@/chunk name." \
        "domain@/subdomain@/item."; do
        if ! grep -F -q -e "$_needle" "$_out"; then
            echo "FAIL $_name: missing [$_needle]" >&2
            cat "$_out" >&2
            exit 1
        fi
    done
    rm -f "$_out" "$_err"
}

check_help "help" "$LITAR" --help
check_help "help wins over -p" "$LITAR" --help -p
check_help "help wins over -l" "$LITAR" --help -l
check_help "help wins over -e" "$LITAR" --help -e
check_help "help wins over -m" "$LITAR" --help -m
check_help "help wins over -d" "$LITAR" --help -d
check_help "help wins over -r" "$LITAR" --help -r
check_help "help wins over -a" "$LITAR" --help -a
check_help "help ignores a missing archive" "$LITAR" --help no-such.la

assert_text "-p" $'OK\n' "$LITAR" -p chunk archive.la
assert_text "--print" $'OK\n' "$LITAR" --print chunk archive.la
assert_text "--print=" $'OK\n' "$LITAR" --print=chunk archive.la
assert_text "archive before -p" $'OK\n' "$LITAR" archive.la -p chunk

lit=$'@<chunk@=\nOK\n@\n'
assert_text "-l" "$lit" "$LITAR" -l chunk archive.la
assert_text "--literal" "$lit" "$LITAR" --literal chunk archive.la
assert_text "--literal=" "$lit" "$LITAR" --literal=chunk archive.la
assert_text "archive before -l" "$lit" "$LITAR" archive.la -l chunk

assert_text "-e" "$lit" "$LITAR" -e chunk archive.la
assert_text "--effective" "$lit" "$LITAR" --effective chunk archive.la
assert_text "--effective=" "$lit" "$LITAR" --effective=chunk archive.la
assert_text "archive before -e" "$lit" "$LITAR" archive.la -e chunk

assert_text "-m" "" "$LITAR" -m / archive.la
assert_text "--modules" "" "$LITAR" --modules / archive.la
assert_text "archive before -m" "" "$LITAR" archive.la -m /
assert_text "-c" $'chunk\n' "$LITAR" -c / archive.la
assert_text "-s" "" "$LITAR" -s C archive.la
assert_text "-i" "" "$LITAR" -i C archive.la

assert_text "-d" "" "$LITAR" -d chunk archive.la
assert_text "--depend" "" "$LITAR" --depend chunk archive.la
assert_text "--depend=" "" "$LITAR" --depend=chunk archive.la
assert_text "archive before -d" "" "$LITAR" archive.la -d chunk

agent=$'[\n    {\n        "comment": "",\n        "labels": [],\n        "module": "",\n        "chunk": "chunk",\n        "type": "set",\n        "content": "OK\\n",\n        "filters": [],\n        "topics": [],\n        "file": "archive.la",\n        "line-range": [1, 3]\n    }\n]\n'
assert_text "-a -l" "$agent" "$LITAR" -a -l chunk archive.la
assert_text "-a -e" "$agent" "$LITAR" -a -e chunk archive.la
assert_text "--agent" "$agent" "$LITAR" --agent -l chunk archive.la
assert_text "agent after the archive" "$agent" "$LITAR" archive.la -l chunk -a
assert_text "agent does not change print" $'OK\n' "$LITAR" -a -p chunk archive.la
assert_text "agent does not change modules" "" "$LITAR" -a -m / archive.la
assert_text "agent depend with no users" $'[]\n' "$LITAR" -a -d chunk archive.la

assert_fail "duplicate -p and --print" "duplicate print option" \
    "$LITAR" -p chunk --print chunk archive.la
assert_fail "duplicate --print=" "duplicate print option" \
    "$LITAR" --print=chunk -p chunk archive.la
assert_fail "duplicate -e and --effective" "duplicate effective option" \
    "$LITAR" -e chunk --effective chunk archive.la
assert_fail "duplicate --effective=" "duplicate effective option" \
    "$LITAR" --effective=chunk -e chunk archive.la
assert_fail "print and effective" "cannot combine print and effective" \
    "$LITAR" -p chunk -e chunk archive.la
assert_fail "effective and print" "cannot combine print and effective" \
    "$LITAR" --effective chunk --print chunk archive.la
assert_fail "literal and effective" "cannot combine literal and effective" \
    "$LITAR" -l chunk -e chunk archive.la
assert_fail "effective and literal" "cannot combine literal and effective" \
    "$LITAR" -e chunk --literal chunk archive.la
assert_fail "effective and modules" "cannot combine modules and effective" \
    "$LITAR" -e chunk -m / archive.la
assert_fail "modules and effective" "cannot combine modules and effective" \
    "$LITAR" -m / --effective chunk archive.la
assert_fail "effective and depend" "cannot combine effective and depend" \
    "$LITAR" -e chunk -d chunk archive.la
assert_fail "depend and effective" "cannot combine effective and depend" \
    "$LITAR" -d chunk --effective chunk archive.la
assert_fail "duplicate -l and --literal" "duplicate literal option" \
    "$LITAR" -l chunk --literal chunk archive.la
assert_fail "duplicate --literal=" "duplicate literal option" \
    "$LITAR" --literal=chunk -l chunk archive.la
assert_fail "print and literal" "cannot combine print and literal" \
    "$LITAR" -p chunk -l chunk archive.la
assert_fail "literal and print" "cannot combine print and literal" \
    "$LITAR" --literal chunk --print chunk archive.la
assert_fail "duplicate -m and --modules" "duplicate modules option" \
    "$LITAR" -m / --modules / archive.la
assert_fail "print and modules" "cannot combine print and modules" \
    "$LITAR" -p chunk -m / archive.la
assert_fail "modules and print" "cannot combine print and modules" \
    "$LITAR" --modules / -p chunk archive.la
assert_fail "literal and modules" "cannot combine modules and literal" \
    "$LITAR" -l chunk -m / archive.la
assert_fail "modules and literal" "cannot combine modules and literal" \
    "$LITAR" -m / --literal chunk archive.la
assert_fail "duplicate -a and --agent" "duplicate agent option" \
    "$LITAR" -a --agent -l chunk archive.la
assert_fail "--agent= is unknown" "unknown option '--agent=foo'" \
    "$LITAR" --agent=foo -l chunk archive.la
assert_fail "agent is not an action" \
    "missing -p, --print, -m, --modules, -c, --chunks, -l, --literal, -e, --effective, -d, --depend, -s, --subdomains, -i, --item, -r, or --reference" \
    "$LITAR" -a archive.la
assert_fail "duplicate -d and --depend" "duplicate depend option" \
    "$LITAR" -d chunk --depend chunk archive.la
assert_fail "duplicate --depend=" "duplicate depend option" \
    "$LITAR" --depend=chunk -d chunk archive.la
assert_fail "print and depend" "cannot combine print and depend" \
    "$LITAR" -p chunk -d chunk archive.la
assert_fail "depend and print" "cannot combine print and depend" \
    "$LITAR" --depend chunk --print chunk archive.la
assert_fail "literal and depend" "cannot combine literal and depend" \
    "$LITAR" -l chunk -d chunk archive.la
assert_fail "depend and literal" "cannot combine literal and depend" \
    "$LITAR" -d chunk --literal chunk archive.la
assert_fail "modules and depend" "cannot combine modules and depend" \
    "$LITAR" -m / -d chunk archive.la
assert_fail "depend and modules" "cannot combine modules and depend" \
    "$LITAR" --depend chunk --modules / archive.la
assert_fail "duplicate -r and --reference" "duplicate reference option" \
    "$LITAR" -r 'C@/item' --reference 'C@/item' archive.la
assert_fail "duplicate --reference=" "duplicate reference option" \
    "$LITAR" --reference='C@/item' -r 'C@/item' archive.la
assert_fail "print and reference" "cannot combine print and reference" \
    "$LITAR" -p chunk -r 'C@/item' archive.la
assert_fail "reference and print" "cannot combine print and reference" \
    "$LITAR" --reference 'C@/item' --print chunk archive.la
assert_fail "literal and reference" "cannot combine literal and reference" \
    "$LITAR" -l chunk -r 'C@/item' archive.la
assert_fail "reference and literal" "cannot combine literal and reference" \
    "$LITAR" -r 'C@/item' --literal chunk archive.la
assert_fail "effective and reference" "cannot combine effective and reference" \
    "$LITAR" -e chunk -r 'C@/item' archive.la
assert_fail "reference and effective" "cannot combine effective and reference" \
    "$LITAR" --reference 'C@/item' -e chunk archive.la
assert_fail "modules and reference" "cannot combine modules and reference" \
    "$LITAR" -m / -r 'C@/item' archive.la
assert_fail "reference and modules" "cannot combine modules and reference" \
    "$LITAR" -r 'C@/item' --modules / archive.la
assert_fail "depend and reference" "cannot combine depend and reference" \
    "$LITAR" -d chunk -r 'C@/item' archive.la
assert_fail "reference and depend" "cannot combine depend and reference" \
    "$LITAR" --reference 'C@/item' -d chunk archive.la
assert_fail "--reference needs an item" \
    "--reference requires an item" "$LITAR" --reference
assert_fail "-r needs an item" "-r requires an item" \
    "$LITAR" -r
assert_fail "empty item" "empty topic name" \
    "$LITAR" --reference= archive.la
assert_text "--modules= names a module" "" \
    "$LITAR" --modules=foo archive.la
assert_text "--chunks= names a module" "" \
    "$LITAR" --chunks=foo archive.la
assert_fail "unknown long option" "unknown option '--bogus'" \
    "$LITAR" --bogus archive.la
assert_fail "unknown short option" "unknown option '-z'" \
    "$LITAR" -z archive.la
assert_fail "missing archive" "missing archive" "$LITAR" -p chunk
assert_fail "missing print" \
    "missing -p, --print, -m, --modules, -c, --chunks, -l, --literal, -e, --effective, -d, --depend, -s, --subdomains, -i, --item, -r, or --reference" \
    "$LITAR" archive.la
assert_fail "missing archive for -m" "missing archive" "$LITAR" -m /
assert_fail "missing archive for --modules" "missing archive" "$LITAR" --modules /
assert_fail "--modules needs a module" "--modules requires a module" \
    "$LITAR" --modules
assert_fail "-m needs a module" "-m requires a module" "$LITAR" -m
assert_fail "--chunks needs a module" "--chunks requires a module" \
    "$LITAR" --chunks
assert_fail "-c needs a module" "-c requires a module" "$LITAR" -c
assert_fail "--subdomains needs a domain" "--subdomains requires a domain" \
    "$LITAR" --subdomains
assert_fail "-s needs a domain" "-s requires a domain" "$LITAR" -s
assert_fail "--item needs a domain" "--item requires a domain" \
    "$LITAR" --item
assert_fail "-i needs a domain" "-i requires a domain" "$LITAR" -i
assert_fail "missing archive for -d" "missing archive" "$LITAR" -d chunk
assert_fail "missing archive for --depend" "missing archive" "$LITAR" --depend chunk
assert_fail "--print needs an expression" "--print requires an expression" \
    "$LITAR" --print
assert_fail "-p needs an expression" "-p requires an expression" \
    "$LITAR" -p
assert_fail "--literal needs a chunk reference" \
    "--literal requires a chunk reference" "$LITAR" --literal
assert_fail "-l needs a chunk reference" "-l requires a chunk reference" \
    "$LITAR" -l
assert_fail "--effective needs a chunk reference" \
    "--effective requires a chunk reference" "$LITAR" --effective
assert_fail "-e needs a chunk reference" "-e requires a chunk reference" \
    "$LITAR" -e
assert_fail "--depend needs a chunk reference" \
    "--depend requires a chunk reference" "$LITAR" --depend
assert_fail "-d needs a chunk reference" "-d requires a chunk reference" \
    "$LITAR" -d
assert_fail "extra argument" "unexpected argument 'extra'" \
    "$LITAR" -p chunk archive.la extra
assert_fail "missing file" "could not open" "$LITAR" -p chunk no-such.la
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
assert_fail "directory is not an archive" "could not read" \
    "$LITAR" -p chunk "$dir"
assert_fail "empty expression" "empty chunk name" \
    "$LITAR" --print= archive.la
assert_fail "filter in expression" "filter 'filt' is not defined in expression" \
    "$LITAR" -p 'chunk@|filt' archive.la
