# Introduction

Programmers have a lot of text to deal with: they write code,
they write code to compile code, they write documents, they write
code to format documents, they write code to parse whatever text,
they even type commands to control computer through shells.
Basically, all work that programmers do is writing text.

The traditional unit of text is file; text of different types
is saved in separate files. Creating a new file is sometimes cumbersome.
Like, you want to insert build-time information to your code whenever you build
your program. To do that, you might write a simple sed script and
put that to Makefile. Here is a problem: sed operates on the whole
file, so you want the sed script to leave the remaining alone.
To solve this, you might move the code being modified to a new file,
use a function to encapsulate it, or simply store the information
in a macro or a variable, then pass it back to the original file.
You can see all the extra efforts made here.

litar operates on a smaller unit of text, which is called chunk.
With litar, you gather text of whatever type to a single file
and describe the relations between chunks with ease; some relations
are hard or cumbersome to implement if between files.
What's more, in the spirit of literate programming,
a litar archive explains itself.
Such a file is called a litar archive; the name litar is a portmanteau of
"literate" and "archive".

# Syntax

Because a litar archive is supposed to contain text of all types, its own syntax
should be as simple as possible. The syntax of a litar archive is built upon
control characters, which are @ followed by a non-alphabetic character.
`@@` is an escape for one ordinary `@`.

A litar archive consists of comments and expressions. Expressions start with a control character,
end with a special control character, @ followed by a space, tab, carriage return, newline,
or the end of the archive.

```
Some comments.

@X expression 1 @

Some other comments.

@X expression 2 @
```

X is a non-alphabetic character.

# Block

Block expressions extend or set a chunk by a piece of text.
Each block expression consists of three basic parts:
the name of a chunk, the operator, and the content of current block.

There are two operators, `@=` and `@+`:
`@=` sets the content of the chunk to the content of the block,
`@+` extends the content of the chunk with the content of the block.

Setting and extending are both ways of applying a block to a chunk.

```
@<chunk1@=
The contents of block1.
@

@<chunk2@=
The contents of block2.
@

@<chunk1@+
The contents of block3.
@

@<chunk2@=
The contents of block4.
@
```

The expanded content of `chunk1` is

```
The contents of block1.
The contents of block3.
```

, but the expanded content of `chunk2` is

```
The contents of block4.
```

A later `@=` overrides the previous content of a chunk.
The final result of a series of applying is a sequence of blocks;
we call the sequence of blocks the effective blocks of the chunk.

Here is another relation between chunk and block: include.

```
Some comments.

@<chunk1@=
The contents of block1.
@

Some other comments

@<chunk2@=
The contents of block2.
@<chunk1@>
The contents of block2.
@

```

block2 contains a reference to chunk1; we say: block2 includes chunk1
at the position of the reference to chunk1.

The content of block is evaluated to

```
The contents of block2.
The contents of block1.
The contents of block2.
```

Let's define formally what the content of a chunk is:
**the result content of all applying in order,
and all references to chunk are replaced by the content of the corresponding chunk**.

It's a recursive procedure to evaluate the content of a chunk,
so be careful to avoid circular dependency.

# Module

The syntax above describes some common concepts that all literate programming tools have.
litar has some useful extensions, which make it a general purpose tool
for text processing rather than only a literate programming tool.

When a litar archive becomes bigger and bigger, despite that the name of a chunk
is usually a long sentence, names might conflict.
So litar supports modularization.

```
@- Module 1 @

@<chunk@=
    The contents.
@

@<another chunk@=
    @<chunk@>
@

@- Module 2 @

@<chunk@=
    @<Module 1@/chunk@>
@
```

The chunk in Module 2 refers to the chunk in Module 1 by preceding the chunk name "chunk"
with the module name "Module 1". Chunks in the same module can ignore the module name.

litar does not stop users from modifying chunks of other modules,
it's a powerful way to interact with other modules in fact, but use this feature
with caution.

Besides the named module, there's an anonymous module. If there's no @-,
or there's no module name specified after @-, the chunks belong to this
anonymous module.

# Specialization

A chunk can be used as a template naturally. Consider that a chunk
defines a general structure for a C program, and we want to write two separated
C programs in single litar archive basing on that.
The final result might look like,

```
@<general structure for C program@=
@<includes@>
int main(int argc, char **argv)
{
    @<main body of C program@>
}
@

@<program1.c@=
@<general structure for C program@>
@

@<program2.c@=
@<general structure for C program@>
@
```

It does not work because all applying to the @<general structure for C program@>
reflects on both programs. litar introduces specialization of chunks.

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

@<program1@:includes@+
#include <stdio.h>
@

@<program1@:main body of C program@=
printf("Hello from program1\n");
@
```

`program1` and `program2` here are called labels. Labels, module name, and chunk name construct a chunk reference.
Extending through a chunk reference does not affect other chunk references of same chunk.
litar allows multiple specializations at once to further specialize a chunk reference.
For example, `@<spec2@:spec1@:module@/chunk@=` specializes `spec1@:module@/chunk` further by spec2,
but it should be pointed out that, `spec2@:spec1@:module@/chunk` does not contain blocks applied to
`spec2@:module@/chunk`.
What `spec2@:spec1@:module@/chunk` specializes is `spec1@:module@/chunk`,
and `spec1@:module@/chunk` does not contain blocks applied to `spec2@:module@/chunk`;
`spec2@:module@/chunk` follows a different path to specialize `module@/chunk`.

After introducing of chunk reference, the evaluation of content is based on chunk references:
**the result content of all applying to the predecessors and the current in order,
and all chunk references are replaced by the content**.

The predecessors of `spec2@:spec1@:module@/chunk` are `spec1@:module@/chunk` and `module@/chunk`.

A sequence of labels can be grouped and assigned a name:

```
@:spec@=spec2@:spec1@
```

# Branching

litar also provides a simple flagging system to support branching.

`@!`: declare a condition;
`@?`: branch to the following arm if the condition is declared;
`@|`: start an arm.

```
@! condition A @

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

The chunk here is extended by block 1 rather than block 2 or block 3.

# Including

Just like programming languages, litar supports factoring out a reusable part.
The reusable part can form a library to be included to other archives.

litar supports C-style including.

'c_structure.la', a library of common C structures:

```
@? included common C structure @|@|

@- C structure @

@<C program@=
@<includes@>
int main(int argc, char **argv) {
    @<main body@>
    return 0;
}
@

@! included common C structure @
@
```

'hello.la', an archive of a C program:

```
@. c_structure.la @

@<hello.c@=
@<hello@:C structure@/C program@>
@

@<hello@:C structure@/includes@+
#include <stdio.h>
@

@<hello@:C structure@/main body@=
printf("Hello\n");
@
```

# Cross Reference

litar allows browsing among related blocks. For example,

- all effective blocks of a chunk reference;
- all blocks that depend on a chunk reference.

These relations help understanding of a litar archive,
but litar does not have knowledge of text it manages.
It does not recognize variable names or function names in programming languages,
nor does it recognize a certain topic of a document.

We can label out these concrete topics explicitly to build underlying connection.

```
Variable 'times' saves the number of occurrences of an event.
@<chunk@=
int times;
@|filter
@~ C@/variables@/times @
```

`@/` means that the topic is under a domain.
litar allows arbitrary further classification of a domain.

# Filter

Another major extension is user defined filters.
The following simple program explains the usage of filters in litar.

```
@<hello.c@=
#include <stdio.h>
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

The final output of this program is "Hello, WORLD!". The creation
of a chunk is pretty easy compared to files, so is the operation to
the chunk. Operations are applied through pipes; an operation is called a filter.
The filters themselves are nothing more than a chunk.
Filter and pipe are borrowed directly from the context of Unix shell.

Filters are scripts that the first line must be a shebang,
so litar can support all script languages without extra effort.

Filters can also be attached to a blocks. The following program print same as the above.

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
```

To arrange multiple filters in one pipeline is possible:
@<print hello world@|exaggeratedly@|more exaggeratedly@>.

The two relations, between block and chunk, are hence extended:

- a block is applied to a chunk reference, the block might be transformed by filters;
- a block includes a chunk reference, the chunk reference might be transformed by filters.

This is the hardest part;
filters are executed by litar, so litar has to
face the complication of building a correct process. For example,
which directory should be the working directory of filters,
how to decide what files a filter can access, etc.

# File System

It's ideal if a filter just use 3 files, stdin, stdout and stderr,
and do nothing more than transforming input from stdin to stdout,
occasionally report error through stderr. In reality,
it's too restrictive. Like Bourne Shell, it cannot do anything meaningful
without accessing tools provided by the underlying operating system.
Even if we limit external programs to a fixed set,
we still have to account for tools like sed that require access to outside script files.

To simply regard filters as executables and run them under the current directory is insufficient too.
what if the invoked sed command wants to make use of a script file written in the litar archive?
Should we extract the file from the litar archive manually then run the sed command?
It's too cumbersome, far from convenient!

The author of litar does not want to limit the powerful flexibility of Unix tools.
So unlike traditional literate programming tools, litar has syntax to introduce
the concept of files explicitly to the kernel of it. This is done by using `@[` and `@(`.

```
@[hello.c@=
#include "stdio.h"
int main() {
    @<print hello world@|exaggeratedly@>
    return 0;
}
@

@<print hello world@=
printf("Hello, world!\n");
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

There are three files in this example, "hello.c", "sed_scripts/bigger_world.sed"
and "run_bigger_world.sh".  The difference between `@[` and `@(` is that `@(` specifies
an executable file but `@[` introduces a regular file.
Filters are executed in an environment so that these three files can be accessed.

The discussion above implies that a litar archive has a file system inside.
To be compatible with Unix programming environment, litar uses a technology called
Filesystem in Userspace (FUSE), which allows litar to create a custom file system
and mount it. So filters can access the files in a litar archive just like
other files in the operating system.

# Invocation

Now it's time to face the real problem in the design of litar.
Introducing of file system causes a lot of consequences.
One major problem is that the circular dependencies are not easy to detect as before.

The expanded content of a chunk depends on not only other chunks it referenced,
but also the filters it invokes. Filters depend heavily on the
file system. For example, a simple 'ls' command will make the output
of the filter depends on all files of the working directory, hence, all chunks these files
relate to. The circular dependency does not only happen more frequently but also is harder to detect.
litar solves this problem by runtime circular dependency detection and explicit file system declaration.

litar requires all invocations of filters to specify which files it can see explicitly.
File set is introduced to ease this work. The following example adds the file hello.c
to two file sets: source files and executables.

```
@[hello.c@`source files@`executables@=
#include <stdio.h>

int main()
{
    printf("Hello\n");
}
@
```

Invocations specify which files they can see using expressions based on file sets.

```
@<list files@=#!/bin/sh
ls
@

@<print source files@=
printf("@<@|list files@+source files@>");
@
```

The expressions support basic set operations. By default, an invocation can not see any file
exported from the archive. @+ adds all files from the file set to the files the invocation
can see, @- subtracts, @, adds a single file, @! deletes a single file.
For example, "@+file set1@-file set2@,file1@,file2@!file3" declares files that
has all files in file set1 but not in file set2 but has file1 and file2 but does not have file3.

litar permits declaring a new file set using the expression.
The previous expression can be used to declare a new file set like following,

```
@`file set@+file set1@-file set2@,file1@,file2@!file3@
```

Keep in mind that litar calculates final files using lazy evaluation.
The calculation happens just before the invocation.

The declaration of file sets requires heavy human intervention,
so it does not work assuredly. In the end, litar should provide helpful
information if circular dependency occurred; users can improve the archive
based on the information.

