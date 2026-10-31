# Introduction

Programmers have a lot of text to deal with: they write code,
they write code to compile code, they write documents, they write
code to format documents, they write code to parse whatever text,
they even type commands to control a computer through shells.
Basically, all work that programmers do is writing text.

The traditional unit of text is the file; text of different types
is saved in separate files. Creating a new file is sometimes cumbersome.
Like, you want to insert build-time information into your code whenever you build
your program. To do that, you might write a simple sed script and
put that to Makefile. Here is a problem: sed operates on the whole
file, so you want the sed script to leave the remaining alone.
To solve this, you might move the code being modified to a new file,
use a function to encapsulate it, or simply store the information
in a macro or a variable, then pass it back to the original file.
You can see all the extra efforts made here.

litar operates on a smaller unit of text, which is called chunk.
With litar, you gather text of whatever type into a single file
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

`X` is a non-alphabetic character.

# Block

Block expressions extend or set a chunk by a piece of text.
Each block expression consists of three basic parts:
the name of a chunk, the operator, and the content of the current block.

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

but the expanded content of `chunk2` is

```
The contents of block4.
```

A later `@=` overrides the previous content of a chunk.
The final result of a series of applying is a sequence of blocks;
we call the sequence of blocks the effective blocks of the chunk.

Here is another relation between chunk and block: containment.

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

`block2` contains a reference to `chunk1`; we say: `block2` contains `chunk1`
at the position of the reference to `chunk1`.

The content of `chunk2` is

```
The contents of block2.
The contents of block1.
The contents of block2.
```

Let's define formally what the content of a chunk is:
**the result of applying every matching block in order,
with each references replaced by the content of that chunk**.

It's a recursive procedure to evaluate the content of a chunk,
so be careful to avoid circular dependency.

# Module

The syntax above describes some common concepts that all literate programming tools have.
litar has some useful extensions, which make it a general purpose tool
for text processing rather than only a literate programming tool.

When a litar archive becomes bigger and bigger, even though a chunk name
is usually a long phrase, names might conflict.
So litar supports modularization.

```
@<module1@/submodule@/chunk@=
    The contents.
@

@<module1@/chunk@=
    @<.@/submodule@/chunk@>
@

@<module2@/chunk@=
    @<module1@/chunk@>
@
```

litar allows attaching a series of module names to a chunk name;
the series of module names, from left to right, form a path like a file path
in a filesystem. Each module name is a directory name, the chunk name is a file name;
a module is a directory, a chunk is a file.

The parent module of all modules is called root module,
written as `/`; it can be omitted usually, but if you must specify it, use this name.
In the content of a block,
you can refer to the module of the chunk reference that the block is applied to as `.`;
`..` is also supported, just as `..` in a file path.
`.` and `..` cannot be used outside the content of a block.

# Specialization

A chunk can be used as a template naturally. Consider that a chunk
defines a general structure for a C program, and we want to write two separate
C programs in a single litar archive based on that.
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

It does not work because all applying to the `general structure for C program`
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
Extending through a chunk reference does not affect other chunk references of the same chunk.
litar allows multiple specializations at once to further specialize a chunk reference.
For example, `@<spec2@:spec1@:module@/chunk@=` specializes `spec1@:module@/chunk` further with the label `spec2`,
but it should be pointed out that, `spec2@:spec1@:module@/chunk` does not include blocks applied to
`spec2@:module@/chunk`.
What `spec2@:spec1@:module@/chunk` specializes is `spec1@:module@/chunk`,
and `spec1@:module@/chunk` does not contain blocks applied to `spec2@:module@/chunk`;
`spec2@:module@/chunk` follows a different path to specialize `module@/chunk`.

After chunk references are introduced, the evaluation of content is based on chunk references:
**the result content of all applying to the predecessors and the current in order,
and all chunk references are replaced by the content**.

The predecessors of `spec2@:spec1@:module@/chunk` are `spec1@:module@/chunk` and `module@/chunk`.

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
of a chunk is pretty easy compared to files, and so is operating on
the chunk. Operations are applied through pipes; an operation is called a filter.
The filters themselves are nothing more than a chunk.
Filter and pipe are borrowed directly from the context of Unix shell.

Filters are scripts whose first line is a shebang,
so litar can support all script languages without extra effort.

Filters can also be attached to a block. The following program prints the same as the above.

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
`@<print hello world@|exaggeratedly@|more exaggeratedly@>`.

The two relations, between block and chunk, are hence extended:

- a block is applied to a chunk reference, the block might be transformed by filters;
- a block contains a chunk reference, the chunk reference might be transformed by filters.

This is the hardest part;
filters are executed by litar, so litar has to
face the complication of building a correct process. For example,
which directory should be the working directory of filters,
how to decide what files a filter can access, etc.

# Cross Reference

litar allows browsing among related blocks. For example,

- all effective blocks of a chunk reference;
- all blocks that depend on a chunk reference.

These relations help with understanding of a litar archive,
but litar does not have knowledge of text it manages.
It does not recognize variable names or function names in programming languages.

We can mark these concrete items explicitly to build underlying connection.

```
Variable 'times' saves the number of occurrences of an event.
@<chunk@=
int times;
@|filter
@~ C@/variables@/times @
```

`@/` means that the item is under a domain.
litar allows arbitrary further classification of a domain.

# File System

It's ideal if a filter uses only three files, stdin, stdout, and stderr,
and does nothing more than transform stdin into stdout,
occasionally reports an error through stderr. In reality,
it's too restrictive. Like Bourne Shell, it cannot do anything meaningful
without accessing tools provided by the underlying operating system.
Even if we limit external programs to a fixed set,
we still have to account for tools like sed that require access to outside script files.

To simply regard filters as executables and run them under the current directory is insufficient too.
What if the invoked sed command wants to make use of a script file written in the litar archive?
Should we extract the file from the litar archive manually then run the sed command?
It's too cumbersome, far from convenient!

The author of litar does not want to limit the powerful flexibility of Unix tools.
So unlike traditional literate programming tools, litar has syntax to introduce
the concept of files explicitly to the kernel of it. This is done by using `@[` and `@(`.

```
@[hello.c@=
#include "stdio.h"
int main() {
    @<print hello world@>
    return 0;
}
@

@<print hello world@=
printf("Hello, world!\n");
@

@(hello.sh@=
#!/bin/sh
echo "Hello, world!"
@
```

There are two files in this example, "hello.c" and "hello.sh".
The difference between `@[` and `@(` is that `@(` specifies
an executable file but `@[` introduces a regular file.

`@[` and `@(` are paired only with `@=`.
The files cannot be specialized or modularized.

The discussion above implies that a litar archive has a file system inside.
To be compatible with Unix programming environment, litar uses a technology called
Filesystem in Userspace (FUSE), which allows litar to create a custom file system
and mount it. So a filter can access an archive file when its view names that file.

# Invocation

Now it's time to face this hard problem in the design of litar.
Introducing a file system causes a lot of consequences.
One major problem is that the circular dependencies are not easy to detect as before.

The expanded content of a chunk depends on not only the other chunks it refers to,
but also the filters it invokes. Filters depend heavily on the file system.
For example, a simple 'ls' command will make the filter's output
depend on all files of the working directory, hence, all chunks these files
relate to. The circular dependency does not only happen more frequently but also is harder to detect.
litar solves this problem by explicit file system declaration.

litar requires all invocations of filters to specify which files they can see explicitly.
File sets are introduced to ease this work. The following example adds the file hello.c
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

The expressions support basic set operations. By default, an invocation cannot see any file
exported from the archive. @+ adds all files from the file set to the files the invocation
can see, @- subtracts, @, adds a single file, @! deletes a single file.
For example, "@+file set1@-file set2@,file1@,file2@!file3" is every file in
`file set1` except the files in `file set2`, plus `file1` and `file2`, except `file3`.

litar calculates final files using lazy evaluation; the calculation happens just before the invocation.
After the calculation, litar mounts a FUSE filesystem and runs the filter under the mountpoint.
The FUSE filesystem has several characteristics:

- it contains the files specified as above, all files are read-only;
- it behaves as an overlay filesystem, a filter might create files,
  litar creates a temporary directory in the host filesystem,
  and maps all writing to the temporary directory.

In the end, litar shall notice and provide helpful information if a circular dependency occurs.
Users can use the information to limit the files that an invocation relates.

# Composing

Just like programming languages,
litar supports composing a bigger archive from smaller ones;
but as we can see, a litar archive is simply a list of blocks,
so the composing of an archive is just concatenating the smaller ones one by one.
Composing archives follows the same pattern as building up of an archive:
blocks are added in order.

litar uses a composer file to express this structure.
The structure for a composed archive is like,

```
composer name/ (a directory)
   composer.json (the composer file)
   archive1
   archive2
   sub-directory/
       archive3
       archive4
```

Here is an example:

```
examples/
    composer.json
    c-structure.la
    hello.la
    AplusB.la
```

'c-structure.la', a library of common C structures:

```
@<C structure@/C program@=
@<C structure@/includes@>
int main(int argc, char **argv) {
    @<.@/main body@>
    return 0;
}
@
```

'hello.la', an archive of a C program:

```
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

'AplusB.la', another C program:

```
@<AplusB.c@=
@<AplusB@:C structure@/C program@>
@

@<AplusB@:C structure@/includes@+
#include <stdio.h>
@

@<AplusB@:C structure@/main body@=
{
    int a, b;
    scanf("%d%d", &a, &b);
    printf("%d\n", a+b);
}
@
```

A composer file is in json format:

```
{
    "ground": "common",
    "nodes": [
        {
            "name": "Basic C Structure",
            "archive": "c-structure.la",
            "description": "Build common patterns that appear in C programs.",
            "mappings": [],
            "base": null
        },
        {
            "name": "Program Hello",
            "archive": "hello.la",
            "description": "Write the classic example in the first lesson of programming.",
            "mappings": [],
            "base": "Basic C Structure"
        },
        {
            "name": "Program A+B",
            "archive": "AplusB.la",
            "description": "Develop a program that reads two numbers then show the result of adding them.",
            "mappings": [],
            "base": "Basic C Structure"
        }
    ]
}
```

The `ground` means a composed archive which this composed archive is based on;
all nodes in `ground` precede the nodes in `examples`. The `ground` is nullable.

Each record in this array is called a node.
A node has a base node; the root node's base is `null`.
A node's blocks consist of its base node's blocks
followed by the blocks in the archive of the node, the ordering matters.

litar reads this composer file first to build a tree of nodes.
Users can move to a specific node, then evaluate expressions according to that node's blocks.

Some limitations:

- there's only one root node in a composer file;
- node names must be unique;
- the archive name is relative to the composer file;
- two nodes cannot share one archive.

The `mappings` field maps the files outside the archives
to the FUSE filesystem created when an invocation happens.

```
{
    "ground": "common",
    "nodes": [
        {
            "name": "Analysis",
            "archive": "./analysis.la",
            "description": "Analyze the samples.",
            "mappings": [
                {
                    "path": "data/samples.txt",
                    "fileset": ["data"],
                    "origin": "./samples.txt"
                }
            ],
            "base": null
        }
    ]
}
```

The example above adds a file `data/samples.txt` to the node and attaches it to the fileset `data`;
the content of the file is the same as `./samples.txt` relative to the composer file.
The files created like this are read-only too.
A child keeps its base's mappings and can add more.

# Reserved Namespaces

litar reserves some namespaces for saving meta information:

- `Module@/{module path}`,
- `Chunk@/{module path@/chunk}`,
- `Domain@/{domain path}`,
- `Index@/{domain path@/item}`.

They are all chunks of a json object.

`Module@/{module path}`:

```
{
    "description": "<description for the module>"
}
```

`Chunk@/{module path@/chunk}`:

```
{
    "description": "<description for the chunk>"
}
```

`Domain@/{domain path}`:

```
{
    "description": "<description for the domain>"
}
```

`Index@/{domain path@/item}`:

```
{
    "description": "<description for the item>"
}
```

Users can assign a JSON object to these chunks to set up meta information,
this meta information may be used by litar to improve user interface
or influence the behaviour of litar.

