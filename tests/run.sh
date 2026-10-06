#!/bin/bash
# Run every case under tests/cases.
# A case is a directory with one of:
#   driver.sh              bash driver.sh LITAR ROOT
#   argv                   one argument per line, passed to litar
#   expr                   litar -p "$(cat expr)" archive.la
#   NAME.expr              one -p check per file, compared with NAME.out,
#                          NAME.err (stderr substring), and NAME.status
# Successful checks expect empty stderr. A missing .out file expects empty
# stdout. The default status is 0.

set -eu

if [ "$#" -ne 1 ]; then
    echo "usage: run.sh LITAR" >&2
    exit 2
fi

LITAR=$1
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CASEDIR=$ROOT/tests/cases

if [ ! -x "$LITAR" ]; then
    echo "litar binary is not executable: $LITAR" >&2
    exit 1
fi

passed=0
failed=0

finish_case() {
    if [ "$1" -eq 0 ]; then
        echo "ok $2"
        passed=$((passed + 1))
    else
        echo "FAIL $2" >&2
        failed=$((failed + 1))
    fi
}

# check_one name status out-file-or-empty err-file-or-empty cmd...
check_one() {
    _name=$1
    _status=$2
    _out_file=$3
    _err_file=$4
    shift 4
    _got_out=$(mktemp)
    _got_err=$(mktemp)
    _exp_out=$(mktemp)
    set +e
    "$@" >"$_got_out" 2>"$_got_err"
    _st=$?
    set -e
    _bad=0
    if [ "$_st" -ne "$_status" ]; then
        echo "FAIL $_name: exit $_st, expected $_status" >&2
        _bad=1
    fi
    if [ -n "$_out_file" ]; then
        cp "$_out_file" "$_exp_out"
    fi
    if ! cmp -s "$_exp_out" "$_got_out"; then
        echo "FAIL $_name: stdout" >&2
        diff -u "$_exp_out" "$_got_out" >&2 || true
        _bad=1
    fi
    if [ -n "$_err_file" ]; then
        _needle=$(cat "$_err_file")
        if ! grep -F -q -e "$_needle" "$_got_err"; then
            echo "FAIL $_name: stderr missing [$_needle]" >&2
            cat "$_got_err" >&2
            _bad=1
        fi
    elif [ "$_status" -eq 0 ] && [ -s "$_got_err" ]; then
        echo "FAIL $_name: unexpected stderr" >&2
        cat "$_got_err" >&2
        _bad=1
    fi
    if [ "$_bad" -ne 0 ] && [ -s "$_got_err" ] && [ -n "$_err_file" ]; then
        :
    elif [ "$_bad" -ne 0 ] && [ -s "$_got_err" ]; then
        echo "stderr:" >&2
        cat "$_got_err" >&2
    fi
    rm -f "$_got_out" "$_got_err" "$_exp_out"
    return "$_bad"
}

run_expr() {
    _name=$1
    _expr_file=$2
    _out_file=$3
    _err_file=$4
    _status_file=$5
    _expr=$(cat "$_expr_file")
    _status=0
    if [ -n "$_status_file" ]; then
        _status=$(tr -d '[:space:]' < "$_status_file")
    fi
    if check_one "$_name" "$_status" "$_out_file" "$_err_file" \
        "$LITAR" -p "$_expr" archive.la; then
        return 0
    fi
    return 1
}

shopt -s nullglob

for dir in "$CASEDIR"/*/; do
    name=$(basename "$dir")
    if [ -f "$dir/driver.sh" ]; then
        if bash "$dir/driver.sh" "$LITAR" "$ROOT"; then
            finish_case 0 "$name"
        else
            finish_case 1 "$name"
        fi
        continue
    fi

    case_bad=0
    back=$(pwd)
    cd "$dir"

    if [ -f argv ]; then
        args=()
        while IFS= read -r line || [ -n "$line" ]; do
            args+=("$line")
        done < argv
        status=0
        if [ -f status ]; then
            status=$(tr -d '[:space:]' < status)
        fi
        out_file=
        err_file=
        if [ -f out ]; then
            out_file=$dir/out
        fi
        if [ -f err ]; then
            err_file=$dir/err
        fi
        if ! check_one "$name" "$status" "$out_file" "$err_file" \
            "$LITAR" "${args[@]}"; then
            case_bad=1
        fi
    elif [ -f expr ]; then
        out_file=
        err_file=
        status_file=
        if [ -f out ]; then
            out_file=$dir/out
        fi
        if [ -f err ]; then
            err_file=$dir/err
        fi
        if [ -f status ]; then
            status_file=$dir/status
        fi
        if ! run_expr "$name" "$dir/expr" "$out_file" "$err_file" "$status_file"; then
            case_bad=1
        fi
    else
        exprs=("$dir"/*.expr)
        if [ "${#exprs[@]}" -eq 0 ]; then
            echo "FAIL $name: no driver.sh, argv, expr, or .expr files" >&2
            case_bad=1
        fi
        for expr_file in "${exprs[@]}"; do
            base=$(basename "$expr_file" .expr)
            out_file=
            err_file=
            status_file=
            if [ -f "$dir/$base.out" ]; then
                out_file=$dir/$base.out
            fi
            if [ -f "$dir/$base.err" ]; then
                err_file=$dir/$base.err
            fi
            if [ -f "$dir/$base.status" ]; then
                status_file=$dir/$base.status
            fi
            if ! run_expr "$name/$base" "$expr_file" "$out_file" "$err_file" \
                "$status_file"; then
                case_bad=1
            fi
        done
    fi

    cd "$back"
    finish_case "$case_bad" "$name"
done

echo "passed $passed failed $failed"
if [ "$((passed + failed))" -eq 0 ]; then
    echo "no test cases found" >&2
    exit 1
fi
[ "$failed" -eq 0 ]
