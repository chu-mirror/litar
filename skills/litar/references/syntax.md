# litar archive syntax

An archive is comments and expressions.
A control character is `@` followed by a non-alphabetic character,
so `@` before a letter is ordinary text.
An expression starts with a control character
and ends with `@` followed by a space, a tab, or a newline.
Text outside expressions is a comment.

A name runs until the next control sequence.
Names in the design are words separated by spaces
(`main body of C program`, `source files`).

## Control sequences

| Sequence | Where it appears | Meaning |
| --- | --- | --- |
| `@` + space, tab, or newline | end of an expression | Ends the expression |
| `@<` | block or reference | Starts a chunk name |
| `@=` | block | Starts the block body |
| `@=` | label group | Separates the group name from the label sequence |
| `@>` | reference | Ends the reference |
| `@:` | qualified name | Label, then the rest of the reference |
| `@:` | start of an expression | Names a sequence of labels |
| `@/` | qualified name | Module, then the chunk |
| `@|` | reference, block tail, or branch | Names a filter, or introduces a branch arm |
| `@-` | start of an expression | Selects the module named after it |
| `@-` | visibility expression | Removes every file in a file set |
| `@!` | start of an expression | Sets a flag |
| `@!` | visibility expression | Removes one file |
| `@?` | branch | Arm kept when its flag is set |
| `@.` | start of an expression | Includes another archive |
| `@[` | file block | Defines a regular file |
| `@(` | file block | Defines an executable file |
| `` @` `` | file block or file-set expression | Names a file set |
| `@+` | visibility expression | Adds every file in a file set |
| `@,` | visibility expression | Adds one file |

## Chunks and blocks

A block names a chunk and supplies content.
The chunk is extended by that block:

```
@<chunk@=
content
@
```

Several blocks may extend one chunk.
The chunk's content is those blocks' expanded contents,
concatenated in archive order.

A reference includes a chunk at that point:

```
@<chunk@>
```

The expanded content of a block is its content
with every reference replaced by the referenced chunk's content.
Evaluation is recursive.
A block cannot include the chunk that it extends.

```
@<chunk1@=
The contents of block1.
@

@<chunk2@=
The contents of block2.
@<chunk1@>
The contents of block2.
@
```

`chunk1` is extended by its block.
`chunk2` includes `chunk1` where `@<chunk1@>` stands.

## Qualified names

Labels, a module name, and a chunk name construct a chunk reference.
The design writes them in this order:

```
@<chunk@>
@<label@:chunk@>
@<module@/chunk@>
@<label@:module@/chunk@>
@<label@:label@:module@/chunk@>
```

The same spelling is used after `@<` in a block (`@=`).
These are combinations the design shows:

```
@<program1@:general structure for C program@>
@<Module 1@/chunk@>
@<hello@:C structure@/C program@>
@<hello@:C structure@/includes@=
@<spec2@:spec1@:module@/chunk@=
```

## Specialization

`program1` and `program2` are labels.
Extending a labeled reference leaves the original chunk unchanged.

```
@<general structure for C program@=
@<includes@>
int main(int argc, char **argv)
{
    @<main body of C program@>
}
@

@<program1.c@=
@<program1@:general structure for C program@>
@

@<program2.c@=
@<program2@:general structure for C program@>
@

@<program1@:includes@=
#include <stdio.h>
@

@<program1@:main body of C program@=
printf("Hello from program1\n");
@
```

`@<program1@:includes@=` and `@<program1@:main body of C program@=`
fill the holes of `@<program1@:general structure for C program@>`.
The original `includes` and `main body of C program` chunks stay unchanged,
so the same template can be labeled again with different text.

Several labels may be applied at once.
`@<spec2@:spec1@:module@/chunk@=`
specializes `spec1@:module@/chunk` further by `spec2`.

A sequence of labels can be grouped and assigned a name:

```
@:spec@=spec2@:spec1@
```

Here `spec` names the sequence `spec2@:spec1`.

## Modules

```
@- Module 1 @
```

Chunks after a `@-` expression belong to the named module
until the next `@-` expression.
With no `@-` yet, or with no name after `@-`,
they belong to the anonymous module.

```
@- Module 1 @

@<chunk@=
The contents.
@

@- Module 2 @

@<chunk@=
@<Module 1@/chunk@>
@
```

Each module has its own `chunk`.
The design allows a block to modify a chunk of another module
through a qualified name,
and treats that as something to use carefully.

## Branching

```
@! condition A @
```

sets a flag.

```
@? condition A @|

@<chunk@=
block 1
@

@? condition B @|

@<chunk@=
block 2
@

@|

@<chunk@=
block 3
@

@
```

`@? condition @|` introduces the arm kept when that flag is set.
`@|` introduces the arm kept when no `@?` flag in the chain is set.
The closing `@` ends the chain.
In this example `condition A` is set,
so `chunk` is extended by block 1.

## Including an archive

A library archive is included by name:

```
@. c_structure.la @
```

The included library selects a module and guards its body with a flag:

```
@? included common C structure @|@|

@! included common C structure @

@- C structure @

@<C program@=
@<includes@>
int main(int argc, char **argv) {
    @<main body@>
    return 0;
}
@

@
```

`@? included common C structure @|`
opens the arm kept when that flag is set, and that arm is empty.
The next `@|` opens the arm kept when the flag is not set.
That arm sets the flag, selects the module `C structure`,
and defines `C program`.
The closing `@` ends the branch.
A later inclusion finds the flag set and keeps the empty arm.

The including archive then extends the library's chunks under its own label:

```
@<hello.c@=
@<hello@:C structure@/C program@>
@

@<hello@:C structure@/includes@=
#include <stdio.h>
@

@<hello@:C structure@/main body@=
printf("Hello\n");
@
```

## Filters

A filter is a chunk whose first line is a shebang.
`@|` pipes text through that chunk the way a Unix shell pipe does.
The filter's output replaces the piped text.

On a reference, the named chunk is filtered, then included:

```
@<hello.c@=
#include "stdio.h"
int main() {
    @<print hello world@|exaggeratedly@>
    return 0;
}
@

@<print hello world@=
printf("Hello, world!\n");
@

@<exaggeratedly@=#!/bin/sh
sed 's/world/WORLD/'
@
```

`@<print hello world@|exaggeratedly@>`
becomes `printf("Hello, WORLD!\n");`.

On a block, the body is filtered, then extends the chunk.
This archive has the same result as the one above:

```
@<hello.c@=
#include "stdio.h"
int main() {
    @<print hello world exaggeratedly@>
    return 0;
}
@

@<print hello world exaggeratedly@=
printf("Hello, world!\n");
@|exaggeratedly@

@<exaggeratedly@=#!/bin/sh
sed 's/world/WORLD/'
@
```

`@|exaggeratedly` filters the body
before that body extends `print hello world exaggeratedly`.
The reference includes the filtered text `printf("Hello, WORLD!\n");`.

With filters, the two relations are:

1. a chunk is extended by a block transformed by filters;
2. a block includes a chunk transformed by filters where the reference stands.

A filter runs as a process.
Which files it can see is declared with the file-set syntax below.
Cycles that involve filters and files are detected when the filter runs.

## Files

`@[` defines a regular file.
`@(` defines an executable file.
The path is the chunk name.
A file is a chunk and can be referenced with `@<`.

```
@[hello.c@=
#include "stdio.h"
int main() {
    @<print hello world@|exaggeratedly@>
    return 0;
}
@

@[sed_scripts/bigger_world.sed@=
s/world/WORLD/
@

@(run_bigger_world.sh@=#!/bin/sh
sed -f sed_scripts/bigger_world.sed
@

@<exaggeratedly@=#!/bin/sh
./run_bigger_world.sh
@
```

`hello.c` and `sed_scripts/bigger_world.sed` are regular files.
`run_bigger_world.sh` is executable.
litar mounts the archive's files with FUSE
so a filter can open them as ordinary files.
`exaggeratedly` is the filter chunk that runs the executable.

## File sets

A file definition may add that file to file sets.
Repeat `` @`name `` once per set:

```
@[hello.c@`source files@`executables@=
#include <stdio.h>

int main()
{
    printf("Hello\n");
}
@
```

This adds `hello.c` to `source files` and to `executables`.

An invocation names the files it can see.
It starts from none of the files exported from the archive.
The operators then apply in order:

- `@+` adds every file in a file set
- `@-` removes every file in a file set
- `@,` adds one file
- `@!` removes one file

```
@<list files@=#!/bin/sh
ls
@

@<print source files@=
printf("@<@|list files@+source files@>");
@
```

`@<@|list files@+source files@>`
invokes the filter `list files`
and lets that invocation see the files in `source files`.

`@+file set1@-file set2@,file1@,file2@!file3`
means the files in `file set1`, without the files in `file set2`,
plus `file1` and `file2`, without `file3`.

The same expression can define a file set:

```
@`file set@+file set1@-file set2@,file1@,file2@!file3@
```

The resulting files are calculated lazily, just before the invocation.
