# Introduction

Programmers have a lot of text to deal with. They write code,
they write code to compile code, they write document, they write
code to format document, they write code to parse whatever text,
they even type commands to control computer through shells.
Basically, all work that programmers do is writing text.

The traditional unit of text is file, text of different types
is saved in separate files, but creating a new file can sometimes be cumbersome.
Like, you want to insert build-time information to your code whenever you build
your program. To do that, you might write a simple sed script and
put that to Makefile. Here is a problem, sed operates on the whole
file, so you have to make sure that the sed script won't influence the other
parts of the file. To solve that problem, you might move that part of code to a new file,
use a function to encapsulate it or simply store the information
in a macro or a variable and pass back to the original file.
You can see all the extra efforts made here.

litar is created to operate on a smaller unit of text, which is called chunk.
With litar, you can gather text of whatever type to a single file
and describe the relations between chunks with ease. Some relations
are hard or cumbersome to implement if between files.
What's more, follow the spirit of literate programming,
the readability is supposed to be improved a lot.
Such a file is called a litar archive, the name litar is a portmanteau of
"literate" and "archive".


# Basic structure

Because a litar archive is supposed to contain text of all types, its own syntax
should be as simple as possible. The syntax of a litar archive is built upon
control characters, which are @ followed by a non-alphabetic character.

A litar archive consists of comments and expressions. Expressions start with a control character,
end with a special control character @ followed by a space character.
The space character can be either space, tab or newline.

```
Some comments.

@... expression 1 ...@

Some other comments.

@... expression 2 ...@
```

# Block

To fully describe all concepts, we shall introduce another term, block.
Each block consists of two basic parts: the name of a chunk and the content of current block.

```
@<chunk1@=
    The contents of block1.
@

@<chunk2@=
    The contents of block2.
@

@<chunk1@=
    The contents of block3.
@

```

We say here that chunk1 is extended by block1 and block3, chunk2 is extended by block2.

Here is another relation between chunk and block, include.

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

The contents of block2 contain a reference to chunk1, we say block2 includes chunk1
at the position of the reference to chunk1.

The definition for the content of a chunk is the expanded content of all
blocks appended to the chunk concatenated.
The definition for the expanded content of a block is the content of the block
that all references to chunk are replaced by the content of the corresponding chunk.

It's a recursive procedure to evaluate the content of a chunk.
So a block cannot include the chunk that it extends.

# Filter

The syntax above describes some common concepts that all literate programming tools have.
litar has some useful extensions, which make it a general purpose tool
for text processing rather than a deliberate literate programming tool.
As a result, litar will not incorporate weaving to its core.

The major extension is user defined filters.
The following simple program explains the usage of filter in litar.

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

The final output of this program is "Hello, WORLD!". The creation
of a chunk is pretty easy compared to files, so is the operation to
the chunk. Operations are applied through pipes,
and filters themselves are nothing more than a chunk. An operation like this is called filter.
Filter and pipe have the same meaning as in the context of Unix shell.

Filters are scripts that the first line must be a shebang,
so litar can support all script languages without extra effort.

Filters can also make effect on blocks. The following program is same with the above.

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

The two relations are extended to,

1. a chunk is extended by a block transformed by filters;
2. a block includes a chunk transformed by filters
   at the position of reference to the chunk.

The whole design of litar is around this extension, all other features are
supplementary to this, and this is the hardest part.
Filters are executed by litar, so litar has to
face the complication of building a correct process. For example,
which directory should be the working directory of filters,
how to decide what files a filter can access, etc.

# File System

It's ideal if a filter just use 3 files, stdin, stdout and stderr,
and do nothing more than transform input from stdin to stdout,
occasionally report error through stderr. In reality,
it's too restrictive. Like Bourne Shell, it can not do anything meaningful
without accessing tools provided by the underlying operating system.
Even if we limited external programs to a fixed set,
we would still have to account for tools like sed that require access to separate script files.
So, to simply regard filters as executables and run them under
current directory is enough? No, what if the invocated sed command want
to make use of a script file written in the litar archive?
Should we extract the file from the litar archive manually then run the sed command?
It's too cumbersome, far from convenient!

The author of litar does not want to limit the powerful flexibility of Unix tools.
So unlike traditional literate programming tools, litar has syntax to introduce
the concept of files explicitly to the kernel of it. This is done by using @[ and @(.

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
and "exaggeratedly". The difference between @[ and @( is that @( specifies
an executable file but @['s is a regular file.
Filters are executed in an environment so that these three
files can be accessed.
It's worthy to mention that a file in a litar archive is also a chunk,
and it can be referenced as usual chunks.

The discussion above implies that a litar archive has a file system inside.
To be compatible with Unix programming environment, litar uses a technology called
Filesystem in Userspace (FUSE), which allows litar to create a custom file system
and mount it. So filters can access the files in a litar archive just like
other files in the operating system.

# Invocation

Now it's time to face the real problem in the design of litar.
The introducing of file system causes a lot of consequences.
One major problem is that the circular dependencies are not easy to detect as before.

The expanded content of a chunk depends on not only other chunks it referenced,
but also the filters it invocates. Filters depends heavily on the
file system. For example, a simple 'ls' command will make the output
of the filter depends on all files of the working directory, hence, all chunks these files
relate to. The circular dependency does not only happen more frequently but also is harder to detect.
litar solve this problem by runtime circular dependency detection and explicit file system declaration.

litar requires all invocations of filters to specify which files it can see explicitly.
File set is introduced to ease this work. The following example adds the file hello.c
to two file sets, source files and executables.

```
@[hello.c@`source files@`executables@=
#include <stdio.h>

int main()
{
    printf("Hello\n");
}
@
```

Invocations can specify which files they can see using expressions based on file sets.

```
@<list files@=#!/bin/sh
ls
@

@<print source files@=
printf("@<@|list files@+source files@>");
@
```

The expressions support basic set operations. By default, no invocation can see any file
exported from the archive. @+ adds all files from the file set to the files the invocation
can see, @- subtracts, @, adds a single file, @! deletes a single file.
For example, "@+file set1@-file set2@,file1@,file2@!file3" declares files that
has all files in file set1 but not in file set2 but has file1 and file2 but does not have file3.

litar allows declaring a new file set using the expression. The previous expression
can be used to declare a new file set like following,

```
@`file set@+file set1@-file set2@,file1@,file2@!file3@
```

Keep in mind that litar calculates final files using lazy evaluation.
The calculation happens just before the invocation.

# Specialization

There's another important extension for reusability of code. Let's say, a chunk
defines a general structure for a C program. Then we want to write two separated
C program in one litar archive. The final result might look like,

```
@<general structure for C program@=
@<includes@>
int main(int argc, char *argv)
{
    @<main body of C program@>
}
@

@[program1.c@=
@<general structure for C program@>
@

@[program2.c@=
@<general structure for C program@>
@
```

It does not work because all extending to the @<general structure for C program@>
reflects on both programs. litar introduce specialization of chunks.

```
@<general structure for C program@=
@<includes@>
int main(int argc, char *argv)
{
    @<main body of C program@>
}
@

@[program1.c@=
@<program1@:general structure for C program@>
@

@[program2.c@=
@<program2@:general structure for C program@>
@

@<program1@:includes@=
#include <stdio.h>
@

@<program1@:main body of C program@=
printf("Hello from program1\n");
@
```

Extending through the specialized name does not affect the original chunk.

# Module

When a litar archive becomes bigger and bigger, despite that the name of a chunk
is usually a long sentence, names might conflict.
So litar supports modularization.

```
@- Module 1

@<chunk@=
    The contents.
@

@- Module 2

@<chunk@=
    @<Module 1@/chunk@>
@
```

litar does not stop users from modifying chunks of other modules, in fact,
it's a powerful way to interact with other modules, but use this feature
with caution.

Besides the named module, there's a anonymous module. If there's no @-,
or there's no module name specified after @-, the chunks belong to this
anonymous module.

