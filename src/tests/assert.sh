# Helpers for tests/cases/*/driver.sh. Source this file; do not execute it.
# assert_* print a diagnostic and exit 1 on failure.

assert_ok() {
    _name=$1
    _exp=$2
    shift 2
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
        echo "FAIL $_name: unexpected stderr" >&2
        cat "$_err" >&2
        exit 1
    fi
    if ! cmp -s "$_exp" "$_out"; then
        echo "FAIL $_name: stdout" >&2
        diff -u "$_exp" "$_out" >&2 || true
        exit 1
    fi
    rm -f "$_out" "$_err"
}

assert_text() {
    _name=$1
    _text=$2
    shift 2
    _file=$(mktemp)
    printf '%s' "$_text" >"$_file"
    assert_ok "$_name" "$_file" "$@"
    rm -f "$_file"
}

assert_fail() {
    _name=$1
    _needle=$2
    shift 2
    _out=$(mktemp)
    _err=$(mktemp)
    set +e
    "$@" >"$_out" 2>"$_err"
    _st=$?
    set -e
    if [ "$_st" -eq 0 ]; then
        echo "FAIL $_name: expected failure" >&2
        exit 1
    fi
    if ! grep -F -q -e "$_needle" "$_err"; then
        echo "FAIL $_name: stderr missing [$_needle]" >&2
        cat "$_err" >&2
        exit 1
    fi
    if [ -s "$_out" ]; then
        echo "FAIL $_name: stdout not empty" >&2
        cat -A "$_out" >&2
        exit 1
    fi
    rm -f "$_out" "$_err"
}
