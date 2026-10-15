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
2. directories in $LITAR_INCLUDE, follow the format with $PATH;
3. $HOME/.litar;
4. $HOME/.local/share/litar;
5. /usr/local/share/litar;
6. /usr/share/litar.

Printing the blocks literally means, for each block, printing the comment before the block,
the chunk reference that the block extends, the content of the block,
and the filters applied to the block.  The printed text is literally a part of the litar archive.
In which module the extending happens will be indicated in the output.

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
@<label@:chunk@=
@<substructure 2@>
@
```

if we run `litar -l label@:chunk` for this, it should print,

```
@- module 1 @

@<chunk@=
@<substructure 1@>
@

@- module 2 @

Extend chunk through label.
@<label@:chunk@=
@<substructure 2@>
@
```

If the extension happens in the anonymous module, print `@-@` to indicate that.

