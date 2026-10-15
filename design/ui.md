litar is a command line tool that supports both short options and long options.

```
usage: litar [options] ARCHIVE

--help: print the usage
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
-l, --literal CHUNK_REFERENCE: print the blocks extending CHUNK_REFERENCE literally
-m, --modules: list all modules
```

The format of EXPRESSION follows "label1@:label2@:module name@/chunk name@|filter1@|filter2".

litar searches the included libraries in this order:

1. current directory;
2. directories in $LITAR_INCLUDE, in the same format as $PATH;
3. $HOME/.litar;
4. $HOME/.local/share/litar;
5. /usr/local/share/litar;
6. /usr/share/litar.

Printing the blocks literally means, for each block, printing the comment before the block,
the chunk reference that the block extends (do not omit the labels and the module name),
the content of the block, and the filters applied to the block.

When the literally printed chunk reference is specialized,
all the blocks extending the less specialized form of that chunk will be printed too.
For example,

```
@- module 1 @

@<chunk@=
@<substructure 1@>
@

@<chunk2@=
@<substructure@>
@

@- module 2 @

Extend chunk through label.
@<label@:module 1@/chunk@=
@<substructure 2@>
@
```

if we run `litar -l label@:module 1@/chunk` for this, it should print,

```
@<label@:module 1@/chunk@=
@<label@:substructure 1@>
@

Extend chunk through label.
@<label@:module 1@/chunk@=
@<substructure 2@>
@
```

