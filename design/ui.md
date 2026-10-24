litar is a command line tool that supports both short options and long options.

```
usage: litar [options] ARCHIVE

--help: print the usage
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
-m, --modules: list all modules
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference TOPIC: list all blocks that refer to TOPIC
-a, --agent: alter output to suit an agent's needs
```

The formats:

- EXPRESSION: `label1@:label2@:module name@/chunk name@|filter1@|filter2`;
- CHUNK_REFERENCE: `label1@:label2@:module name@/chunk name`;
- TOPIC: `domain@/subdomain@/item`.

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
all the blocks applied to the predecessors of that chunk reference will be printed too.
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

If we run `litar -l label@:module 1@/chunk` for this, it should print,

```
@<module 1@/chunk@=
@<substructure 1@>
@

Extend chunk through label.
@<label@:module 1@/chunk@=
@<substructure 2@>
@
```

A block depending on a chunk reference means the content of the block contains, or is filtered by,
the chunk reference or a chunk reference specialized from the chunk reference.

```
@<chunk 1@=
content
@

@<chunk 2@=
@<label 1@:chunk 1@>
@

@<chunk 3@=
content
@|label 2@:chunk 2@
```

The second block depends on `chunk 1` and `label 1@:chunk 1`;
the third block depends on `chunk 2` and `label 2@:chunk 2`.

The format of output of option `-e`, `-d`, and `-r` agrees with `-l`.

Consult [agent.md](agent.md) for the behaviour of `-a`/`--agent` option.
