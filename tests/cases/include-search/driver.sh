#!/bin/bash
set -eu
LITAR=$1
ROOT=$2
. "$ROOT/tests/assert.sh"

tmp=$(mktemp -d)
cleanup() {
    if [ -f "$tmp/cwd/hidden.la" ]; then
        chmod u+r "$tmp/cwd/hidden.la" || true
    fi
    rm -rf "$tmp"
}
trap cleanup EXIT

name=uniq-stage0-lib.la
mkdir -p "$tmp/cwd" "$tmp/inc" "$tmp/home/.litar" "$tmp/home/.local/share/litar"
printf '@<k@=\nCWD\n@\n' >"$tmp/cwd/$name"
printf '@<k@=\nINC\n@\n' >"$tmp/inc/$name"
printf '@<k@=\nHOME\n@\n' >"$tmp/home/.litar/$name"
printf '@<k@=\nSHARE\n@\n' >"$tmp/home/.local/share/litar/$name"
printf '@. %s @\n' "$name" >"$tmp/cwd/main.la"
cd "$tmp/cwd"

assert_text "cwd first" "CWD" \
    env HOME="$tmp/home" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la
rm -f "$name"
assert_text "LITAR_INCLUDE" "INC" \
    env HOME="$tmp/home" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la
rm -f "$tmp/inc/$name"
assert_text "HOME/.litar" "HOME" \
    env HOME="$tmp/home" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la
rm -f "$tmp/home/.litar/$name"
assert_text "HOME share" "SHARE" \
    env HOME="$tmp/home" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la
rm -f "$tmp/home/.local/share/litar/$name"
assert_fail "not found" "could not find archive '$name'" \
    env HOME="$tmp/home" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la

printf '@<k@=\nFILE\n@\n' >"$tmp/inc/$name"
mkdir -p "$name"
assert_text "directory skipped" "FILE" \
    env HOME="$tmp/nowhere" LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k main.la
rmdir "$name"

assert_text "colon list" "FILE" \
    env -u HOME LITAR_INCLUDE="/no/such/litar-dir:$tmp/inc:" \
    "$LITAR" -p k main.la

printf '@<k@=\nDOT\n@\n' >"$name"
assert_text "trailing colon" "DOT" \
    env -u HOME LITAR_INCLUDE="/no/such/litar-dir:" "$LITAR" -p k main.la
assert_text "empty component" "DOT" \
    env -u HOME LITAR_INCLUDE=":/no/such/litar-dir" "$LITAR" -p k main.la

mkdir -p "$tmp/abs"
printf '@<k@=\nABS\n@\n' >"$tmp/abs/real.la"
printf '@<k@=\nCWDABS\n@\n' >"$tmp/cwd/real.la"
printf '@. %s @\n' "$tmp/abs/real.la" >"$tmp/cwd/abs.la"
assert_text "absolute" "ABS" "$LITAR" -p k abs.la
printf '@. %s @\n' "$tmp/abs/nope.la" >"$tmp/cwd/absmiss.la"
assert_fail "absolute missing" "could not find archive '$tmp/abs/nope.la'" \
    "$LITAR" -p k absmiss.la

printf '@<k@=\nSECRET\n@\n' >"$tmp/cwd/hidden.la"
chmod 000 "$tmp/cwd/hidden.la"
printf '@<k@=\nLATER\n@\n' >"$tmp/inc/hidden.la"
printf '@. hidden.la @\n' >"$tmp/cwd/hidemain.la"
assert_fail "unreadable" "could not open" \
    env -u HOME LITAR_INCLUDE="$tmp/inc" "$LITAR" -p k hidemain.la
chmod u+r "$tmp/cwd/hidden.la"

rm -f "$name"
printf '@<k@=\nHOME\n@\n' >"$tmp/home/.litar/$name"
assert_text "HOME set" "HOME" \
    env -u LITAR_INCLUDE HOME="$tmp/home" "$LITAR" -p k main.la
assert_fail "HOME unset" "could not find archive '$name'" \
    env -u HOME -u LITAR_INCLUDE "$LITAR" -p k main.la
assert_fail "HOME empty" "could not find archive '$name'" \
    env -u LITAR_INCLUDE HOME="" "$LITAR" -p k main.la
