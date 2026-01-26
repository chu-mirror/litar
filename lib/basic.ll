@- Basics

@<evaluate@=#!/bin/sh
set -e
temp_file=`mktemp`
cat - > $temp_file
chmod +x $temp_file
$temp_file
rm $temp_file
@

@[helper.md@=
Help
@
