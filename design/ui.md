litar is a command line tool, supports both short options and long options.

```
usage: litar [options] ARCHIVE

--help: print the usage
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
-l, --literal CHUNK_REFERENCE: print the blocks extending CHUNK_REFERENCE literally
```

The format of EXPRESSION follows "label1@:label2@:module name@/chunk name@|filter1@|filter2".

litar searches the included libraries in this order:

1. current directory;
2. directories in $LITAR_INCLUDE, follow the format with $PATH;
3. $HOME/.litar;
4. $HOME/.local/share/litar;
5. /usr/local/share/litar;
6. /usr/share/litar.

When the literally printed chunk reference is specialized,
the labels should be passed to the chunk references included by the printed chunk.
For example,

```
@<chunk@=
@<substructure 1@>
@

@<label@:chunk@=
@<substructure 2@>
@
```

if we run `litar -l label@:chunk` for this, it should print,

```
@<label@:substructure 1@>
@<substructure 2@>
```

