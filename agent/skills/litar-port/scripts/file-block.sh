#!/bin/bash
# Print litar file blocks that reproduce project files byte for byte.
#
# usage: file-block.sh [-C PROJECT] [-s SET]... PATH...
#   -C PROJECT  directory the paths are relative to (default: .)
#   -s SET      add every file to file set SET (repeatable)
#
# An executable file becomes @( ... ), any other file @[ ... ].
# Binary files (containing NUL) and files ending in a lone CR are skipped
# with a warning; the exit status is 1 if any file was skipped.
set -u

project=.
sets=()
while getopts "C:s:" opt; do
    case $opt in
        C) project=$OPTARG ;;
        s) sets+=("$OPTARG") ;;
        *) echo "usage: $0 [-C PROJECT] [-s SET]... PATH..." >&2; exit 2 ;;
    esac
done
shift $((OPTIND - 1))
[ $# -gt 0 ] || { echo "usage: $0 [-C PROJECT] [-s SET]... PATH..." >&2; exit 2; }

# Double every @ that is not followed by an ASCII letter or a byte >= 128.
escape() { perl -0777 -pe 's/\@(?![A-Za-z\x80-\xff])/\@\@/g'; }

status=0
for path in "$@"; do
    file=$project/$path
    if [ ! -f "$file" ]; then
        echo "skip $path: not a regular file" >&2; status=1; continue
    fi
    if perl -0777 -ne 'exit(/\0/ ? 0 : 1)' "$file"; then
        echo "skip $path: binary" >&2; status=1; continue
    fi
    if perl -0777 -ne 'exit(/\r\z/ ? 0 : 1)' "$file"; then
        echo "skip $path: ends with a lone CR" >&2; status=1; continue
    fi
    open='@['
    [ -x "$file" ] && open='@('
    printf '%s%s' "$open" "$(printf '%s' "$path" | escape)"
    for s in "${sets[@]+"${sets[@]}"}"; do
        printf '@`%s' "$(printf '%s' "$s" | escape)"
    done
    # The line break after @= and the one before the closing @ are layout.
    printf '@=\n'
    escape < "$file"
    printf '\n@\n'
done
exit $status
