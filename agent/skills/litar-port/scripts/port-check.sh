#!/bin/bash
# Compare files evaluated from a litar archive with the original project.
#
# usage: port-check.sh [-v] [-o OUT] ARCHIVE PROJECT [PATH...]
#   ARCHIVE   the node's archive; litar finds its composition with -f
#   PROJECT   the original project directory
#   PATH      files to check, relative to PROJECT
#             (default: git ls-files, or every file outside .git)
#   -v        show a diff for each mismatch
#   -o OUT    also write every evaluated file under OUT, e.g. to build it
#
# Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
# Exit status 1 if any file differs or fails to evaluate.
set -u

verbose=0
out=
while getopts "vo:" opt; do
    case $opt in
        v) verbose=1 ;;
        o) out=$OPTARG ;;
        *) echo "usage: $0 [-v] [-o OUT] ARCHIVE PROJECT [PATH...]" >&2; exit 2 ;;
    esac
done
shift $((OPTIND - 1))
[ $# -ge 2 ] || { echo "usage: $0 [-v] [-o OUT] ARCHIVE PROJECT [PATH...]" >&2; exit 2; }
archive=$1
project=$2
shift 2
litar=${LITAR:-litar}

if [ $# -eq 0 ]; then
    if git -C "$project" rev-parse >/dev/null 2>&1; then
        mapfile -t paths < <(git -C "$project" ls-files)
    else
        mapfile -t paths < <(cd "$project" && find . -path ./.git -prune -o -type f -print | sed 's|^\./||')
    fi
else
    paths=("$@")
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
ok=0
bad=0
for path in "${paths[@]}"; do
    # A name stops at the next control, so @ in a path must be escaped.
    name=$(printf '%s' "$path" | perl -pe 's/\@(?![A-Za-z\x80-\xff])/\@\@/g')
    got=$tmp/got
    if ! "$litar" -f "$archive" -p "$name" >"$got" 2>"$tmp/err"; then
        echo "FAIL     $path: $(head -n1 "$tmp/err")"
        bad=$((bad + 1))
        continue
    fi
    if [ -n "$out" ]; then
        mkdir -p "$out/$(dirname "$path")"
        cp "$got" "$out/$path"
        [ -x "$project/$path" ] && chmod +x "$out/$path"
    fi
    if cmp -s "$got" "$project/$path"; then
        ok=$((ok + 1))
    else
        echo "MISMATCH $path"
        [ $verbose -eq 1 ] && diff -u "$project/$path" "$got" | head -n 40
        bad=$((bad + 1))
    fi
done
echo "same $ok, different $bad"
[ $bad -eq 0 ]
