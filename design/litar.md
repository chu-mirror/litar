---
title: Literate Programming for Agents
author: Ma Zhizhong
---

litar is rooted in Donald Knuth's
[Literate Programming](http://www.literateprogramming.com/knuthweb.pdf).
Knuth says to write a program as an essay; a program written
in this form will be better and more understandable.
Unfortunately, in the AI era,
most of the authoring work is done by LLMs rather than by humans.
LLMs robbed human beings of the pleasure of writing, sigh.

Anyway, we must adapt to the new paradigm.
Literate Programming shines once again, if it has ever shone,
when the cooperation between humans and LLMs becomes more and more important.

The problem is that LLMs produce output too quickly to examine all the details,
but we still want a way to grasp what LLMs have done.
To command an agent to explain itself is one of the methods;
litar tries to provide a more efficient way for this purpose:
let LLMs write programs in literate program form to make the output more accessible.

# Literate Programming

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

Literate programming operates on a smaller unit of text, which is called a chunk.
You gather text of whatever type into a single file
and describe the relations between chunks with ease;
some relations are hard or cumbersome to implement if between files.
What's more, in the spirit of literate programming,
this construction makes a program more readable.
A file that contains a lot of chunks is called a litar archive;
the name litar is a portmanteau of "literate" and "archive".

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

## Block

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
with each reference replaced by the content of that chunk**.

It's a recursive procedure to evaluate the content of a chunk,
so be careful to avoid circular dependency.

## Module

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
the series of module names, from left to right, form a path like a file path in a filesystem.
Each module name is a directory name, the chunk name is a file name;
a module is a directory, a chunk is a file.

The parent module of all modules is called root module,
written as `/`; it can be omitted usually, but if you must specify it, use this name.
In the content of a block,
you can refer to the module of the chunk reference that the block is applied to as `.`;
`..` is also supported, just as `..` in a file path.
`.` and `..` cannot be used outside the content of a block.

## Specialization

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

`program1` and `program2` here are called labels.
Labels, module name, and chunk name construct a chunk reference.
Extending through a chunk reference does not affect
other chunk references of the same chunk but specialized differently.

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

## Filter

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

This is the hardest part; filters are executed by litar,
so litar has to face the complication of building a correct process.
For example, which directory should be the working directory of filters,
how to decide what files a filter can access, etc.

The rule of evaluation is completed now that filters are introduced,
here is a little addition: evaluation also keeps indentation.
If only spaces and tabs precede a chunk reference on its line,
the content replacing it is indented at the same level:
each of its lines after the first is preceded by the same spaces and tabs.
This little tuning makes the evaluation more intuitive,
and makes programming an indentation based language, Python for instance, less painful.

## Cross Reference

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
@? C@/variables@/times @

@<count once@=
++times;
@~ C@/variables@/times @
```

`@?` means that this block defines the item,
`@~` means that this block refers to the item,
`@/` means that the item is under a domain.
litar allows arbitrary further classification of a domain.
A block may carry several items, each introduced by `@?` or `@~`;
a block carries every item attached to it, and refers to those it does not define.

## File System

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

## Invocation

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

- it contains the files specified as above;
- it behaves as an overlay filesystem, a filter might create files,
  litar creates a temporary directory in the host filesystem,
  and maps all writing to the temporary directory.

In the end, litar shall notice and provide helpful information if a circular dependency occurs.
Users can use the information to limit the files that an invocation relates.

## Reserved Namespaces

litar reserves some namespaces for saving meta information:

- `Chunk@/{chunk path}`,
- `Specialization@/{labels}`,
- `File@/{file path}`,
- `Index@/{item path}`.

They are all chunks of a json object.

`Chunk@/{chunk path}`:

```
{
    "description": "<description for the chunk or module>"
}
```

`Specialization@/{labels}`:

```
{
    "description": "<description for the specialization>"
}
```

The path `{labels}` is organized reversed, for example,
the path for specialization `label2@:label1` is `Specialization@/label1@/label2`.

`File@/{file path}`:

```
{
    "description": "<description for the file or directory>"
}
```

`Index@/{item path}`:

```
{
    "description": "<description for the item or domain>"
}
```

Users can assign a JSON object to these chunks to set up meta information,
this meta information may be used by litar to improve user interface
or influence the behaviour of litar.

# Authoring

The language used to constrain the authoring of a litar archive
is based on the discussion of the shapes of structures.

The knowledge for something is always structured, and programs are the same.
For example, a C source file, at the outermost level,
consists of includes, macros, variables, and functions, etc.
A C source file hence shows itself in a litar archive typically as follows:

```
@[program.c@=
@<includes@>
@<variables@>
@<functions@>
@

@<functions@+
int main(int argc, char **argv)
{
    int ret = 0;
    @<main body@>
    return ret;
}
@

@<main body@=
@<initialize global variables@>
if (ret) {
    return ret;
}
@<read and handle input@>
if (ret) {
    return ret;
}
@<output result@>
@
```

It's clear that a chunk encapsulates a structure,
and there are two shapes of structure found here: sequence and tree.

A structure of sequence shape is formed by a series of substructures of the same type,
for example, a series of include files, a series of functions.
A structure of tree shape is formed by a fixed composition of substructures.

Among the chunks above, `includes`, `variables`, and `functions` are sequences,
`program.c` and `main body` are trees, other chunks are undefined,
but as we see in the following guidelines, we can assume that they are trees from the names.

## Name a chunk with a phrase, whose grammatical role fits that structure

First of all, because the name of a chunk is a phrase,
**do not capitalize the first character and do not append a period at the end**.
And one principle is mandatory:
**name a chunk with a short description of the encapsulated structure**.
It might be unnecessary to stress this principle, but anyway, this principle is the most important.

Naming a sequence is simple; a sequence is always a list of items,
so **name a sequence with a plural noun phrase**.

Naming a tree is much more difficult; it depends heavily on the context.
When programming in imperative style, the expressions usually operate on something,
so a verb phrase is appropriate for encapsulating these expressions.
When programming in functional style, the expressions are simply values,
so a noun phrase is more suitable here.
Even these apparent rules are not always applicable; like the `main body` in the example above,
it's named from a declarative viewpoint.

Nevertheless, there is some advice for more concrete situations.
For imperative expressions, the rules are well established:

- **quote the objects being operated on in the phrase**;
- **all quoted objects shall be declared first before quoting**;
- **omit the quotation if there are many chunks that refer to the same objects**;
- **do not refer to an object declared inside a chunk outside the chunk**;
- **use concrete verbs, do not use verbs like 'perform', 'do'**.

The quoting here means that if a block of code is meant to add two numbers,
`number1` and `number2`, the result is saved as `number3`,
then the chunk name shall be `save the sum of 'number1' and 'number2' to 'number3'`.

Other structures, such as a complicated data structure or a structured document, etc,
shall be named with a noun phrase, **name a tree with a noun phrase if it's not an action**.

## Encapsulate a chunk that shall not be referred to outside the module into a submodule

This rule is derived from the general rule: hide the unnecessary details.
Here is an example that shows each case:

```
@<C Structure@/module@=
@<.@/includes@>
@<.@/definitions@>
@

@<C Structure@/definitions@+
void print_hello()
{
    @<.@/Output@/print hello to stdout@>
}
@

@<C Structure@/Output@/print hello to stdout@=
printf("Hello\n");
@
```

The chunk `includes` and `definitions` are in the same module as `module`,
but `print hello to stdout` is in the submodule `Output`.
Because the former two form a template, when another module use `module` to create
a C module so it must access the two chunks directly, but `print hello to stdout`
is used only by the function `print_hello`, it's not supposed to be accessed outside the module.

## If a tree is big enough, divide it

So how do we determine whether a tree is big enough?

Imagine that you are facing a C source file of thousands of lines without a header file,
and you are trying to figure out what this file does.
A skilled engineer will never read this file line by line.
The steps he follows might be,

1. go straight to functions, find a global function and start reading it;
2. find the data types that the outermost logic deals with;
3. list all global functions touching the same data types;
4. figure out what this group of functions does.

The C source file can be regarded as a structure of tree shape,
its substructures are functions, data types; a global function is also a tree,
it has an outermost logic, the details are encapsulated in substructures.

The actions the skilled engineer takes are searching for functions,
skimming the outermost logic of a function, searching for data types,
and searching for functions that refer to a particular data type.

A main goal of litar is to make the actions based on the structures easier,
and the first step is to make the structures explicit.
So an acceptable answer to the question at the opening is that the tree is long
enough that those actions take a skilled engineer several minutes.
Token count is a good measure of length;
150 tokens is a comfortable length for human beings, at least for the author.
The size of a block is the content of the block. Comments and items are not counted.

In practice, the length of a tree usually equals the length of a block.
Here's a more practical guideline: **control the length of a block at or below about 150 tokens**.

## Explain a decision in the comment

In the process of implementing an idea, we always make some compromises, trade-offs,
or simply some details the idea has not specified. For example, the size of a buffer,
handling of a failure, choosing of algorithm, etc.

These details live outside the specification, scattered over implementation,
but closely affect the finished program.
litar requires these details to be documented where they are introduced.

## Make blocks extending a sequence order-irrelevant

A data type definition consists of other data types;
so a data type should be put after the data types it's based on.
In this case, if the data type and the base data types are in different blocks,
the dependency imposes an order on blocks.

The blocks shall be loosely related to reduce the context needed for understanding or editing a block.
The `includes` in the above example is an ideal order-irrelevant sequence;
if a block is created to introduce more header files,
the only context information is the name of the chunk it ought to extend, `includes` here.
Also, readers do not have to list all the blocks of `includes` to figure out what the current block
is supposed to do.

Sometimes, the ordering is unavoidable. For example, in an if-else chain, once one arm matches,
every later arm is skipped even if its condition would be true.

The rule: **gather substructures of a sequence into a single block if the order matters**.
This rule might conflict with the rule of block length limitation,
if they conflict, prioritize this rule.

## Reduce repetition by filters

A single piece of information can appear at so many places of a project in different forms.
The specification of command line interface is a good example.
It appears in help information, parsing algorithm, and manpage.
It's cumbersome to maintain all three copies,
and these copies can be programmatically generated from the original specification;
filters are a good fit for that generation.

Write the specification of command line interface declaratively in a chunk;
it can be in any format like json, yaml, toml,
data structure in script languages (python, perl, for example), etc.
Then for each form, write a filter to transform the specification,
finally use chunk references to insert the transformed specification into proper places.

The rule: **use filters to generate different forms of a single piece of information**.

## Build up cross-reference when possible

A chunk of code always refers to other concepts, like, calling functions, modifying variables.
Figure out whether the item is local; that is, whether the context of the item is limited to
current block. If not, build reference to that item:
**mark the block that defines the item with `@?`, and every block that uses it with `@~`**.

Here are some rules:

- **write the definition in the reserved namespace `Index@/{item path}`**,
  the description of a concept or a term is its one-line definition,
  so the item tree works as a glossary;
- **explain an item fully in the comment of the block that defines it**,
  every other block only refers to the item;
- **define an item in one block, if it has a definition**,
  a function, a variable, or a concept is defined once,
  but a topic, and usually a term, has no defining block and is only referred to;
- **create an item only when it connects blocks**,
  at least two blocks carry it, or soon will.

Selecting the context of a task (see [Context](#context)) traces *R* edges
from the blocks that refer to an item to the block that defines it,
so an item without a defining block cannot be traced,
and a definition marked with `@~` is taken for one more use.

There are some cases we want to avoid cross-reference:

- the items are from common sense, like standard library, well-known third-party libraries;
- the items are quoted in the name of the chunk name, if a chunk's name is `add 'a' to 'b'`,
  then even though 'a' and 'b' is not limited to current block, the name implies the reference
  implicitly, so do not use cross-reference.

## Do not assign every tree with a chunk, use cross-referencing when appropriate

Some trees are well defined units under the context, for example, a function in programming.
That means, do not write:

```
@<functions@+
@<print hello@>
@

@<print hello@=
void hello()
{
    printf("Hello\n");
}
@
```

write:

```
@<functions@+
void hello()
{
    printf("Hello\n");
}
@? function@/hello @
```

# Category

The Authoring section gives general advice about
how to create chunks and items and when to put a chunk under a module,
but that's not the whole story.
The chunks and items are categorized under modules and domains.
When the scale expands, a firm strategy to classify them becomes important.

The examples above are simplified, so they should not be used as models.

## Chunks

A chunk tree might look like the following:

- Tool: useful filters shared by all modules
- Language: common structures found in languages
  - C
    - Module: C source file and C header file
    - Project: the whole codebase, including build system
  - Go
    - File: Go source file
    - Package: Go package
    - Module: go.mod unit
  - (other languages)
- UI: the shared schema and generators
  - Command Line: the invocation syntax and the documents generated from it
    - C: generators of command line parsing in C
    - Go: generators of command line parsing in Go
  - Terminal
  - Graphical
- Framework: something an application chooses to adopt
  - (frameworks for building applications)
- Project
  - (concrete projects)
    - Build: the build system
    - Tests
    - Documents

Every chunk is free to refer to any other chunk.

## Items

An item path answers three questions in order:
in which context the name means something, what kind of thing it is, and what it is called.

```
<owner>@/<kind>@/<name>
```

The owner is the narrowest context in which the name is unambiguous.
It is written with the same path as the module that owns the chunks of that context,
so chunks and items share one tree:

- `Tool`
- `Language@/Go`
- `UI@/Command Line`
- `Framework@/Light`
- `Project@/litar`

An item moves up to a wider owner only when a second owner starts using it.

Kinds come from a short closed list.
A kind is a plural noun, like the name of a sequence in the Authoring section.
There are two families of kinds.

Code kinds sit under the language,
because they are specific to the language and a project can mix languages:
`Go@/functions`, `Go@/methods`, `Go@/types`, `Go@/variables`, `C@/macros`, etc.

Knowledge kinds have no language level:

- `concepts`: one block defines it, other blocks depend on that definition;
- `terms`: a word given a narrower meaning in this context,
  the thing it names is described elsewhere or needs no definition;
- `topics`: a subject several blocks are about, with no single definition;
- `options`: a switch or setting that users face.

When a name could be a term or a concept, choose `concepts`.

A code item keeps the name as the language qualifies it, such as `litar.run`.
A knowledge item is named by a singular phrase,
not capitalized and without a period, the same as a chunk.

Here are some examples:

```
Project@/litar@/Go@/functions@/litar.run
Project@/litar@/concepts@/effective block
Project@/litar@/terms@/view
Project@/litar@/topics@/filter cache
Project@/litar@/options@/--agent
Framework@/Light@/C@/macros@/LIGHT_TEST
Language@/Go@/concepts@/build constraint
```

# Composing

Just like programming languages,
litar supports composing a bigger archive from smaller ones;
but as we can see, a litar archive is simply a list of blocks,
so the composing of an archive is just concatenating the smaller ones one by one.
Composing archives follows the same pattern as building up of an archive:
blocks are added in order.

The composed archives are called a composition.
litar uses a composer file to express the structure;
the files in a composition are like,

```
composition/ (a directory)
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
    programs/
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

'programs/hello.la', an archive of a C program:

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

'programs/AplusB.la', another C program:

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
    "description": "An example of composition",
    "ground": null,
    "nodes": [
        {
            "name": "Basic C Structure",
            "archive": "c-structure.la",
            "description": "Build common patterns that appear in C programs.",
            "mappings": [],
            "base": []
        },
        {
            "name": "Program Hello",
            "archive": "programs/hello.la",
            "description": "Write the classic example in the first lesson of programming.",
            "mappings": [],
            "base": ["Basic C Structure"]
        },
        {
            "name": "Program A+B",
            "archive": "programs/AplusB.la",
            "description": "Develop a program that reads two numbers then show the result of adding them.",
            "mappings": [],
            "base": ["Basic C Structure"]
        }
    ]
}
```

The `ground` means a composition which this composition is based on;
all nodes in `ground` precede the nodes in `examples`. The `ground` is nullable.
`ground` can be a relative path starting with `.` or `..`,
or a name searched for in a series of directories.

Each record in the array is called a node.
A node has a base node, the base node is expressed as a path from root node down to direct parent node;
the root node's base is an empty list.
A node's blocks consist of its base node's blocks
followed by the blocks in the archive of the node, the ordering matters.

litar reads this composer file first to build trees of nodes.
Users can move to a specific node, then evaluate expressions according to that node's blocks.

Some limitations:

- the final expanded node list must contain only one root node;
- node names under a common parent must be unique;
- the archive name is relative to the composer file;
- two nodes cannot share one archive.

The example above is a bad one. Nodes are derived from a common base
because they contain contradictory blocks, for example, a modification
to the C structures that other C programs must not share. The two C programs here
can coexist in one node. litar permits the temporary use of a node while the placement
of its blocks is undecided.

The `mappings` field maps the files outside the archives
to the FUSE filesystem created when an invocation happens.

```
{
    "description": "An example for mappings",
    "ground": null,
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
            "base": []
        }
    ]
}
```

The example above adds a file `data/samples.txt` to the node and attaches it to the fileset `data`;
the content of the file is the same as `./samples.txt` relative to the composer file.
A child keeps its base's mappings and can add more.

The organization of compositions can mimic branching like nodes,
but the purpose of using several compositions rather than one big composition
is to control who may modify which nodes.
Applying a block to a node in the ground composition is forbidden,
so is writing to mappings in the ground composition.

# Context

As we can see from the categorization above,
litar tries to build inter-connection among different domains,
but it also means that a composition may contain too much unnecessary information for a single task.
The context is too big, for human, also for LLMs.

A well defined context is the key to good understandability,
it shall be as small as possible, but complete, moreover, intriguing.
The flat structure of litar archives makes the management of contexts really easy:
a context for litar is simply a set of blocks,
then the question is reduced to how to select blocks for a task.

All blocks, as Donald Knuth said, are organized as a web.
A block may contain a chunk reference, hence contain all the effective blocks of that chunk reference.
A block may refer to an item defined in other blocks.
All these relations make a graph;
this graph is the theoretical basis for our context management.

The graph is a directed graph, it has two types of edges: contain and refer.
We call them *C* edge, and *R* edge.

- a *C* edge points from A to B if A contains a chunk reference to which B is applied,
  or A contains a chunk reference filtered by a chunk reference to which B is applied,
  the former is also called a *Ca* edge, the latter is called a *Cf* edge;
- an *R* edge points from A to B if A refers to an item which B defines.

Here are some simple strategies based on the graph:

- to figure out what a block is meant to do, visit back the *C* edges;
- to figure out what a block does, visit through the *C* edges and *R* edges;
- to figure out how to deal with an item, visit back the *R* edges.

A task can be anything like,

- exploring a codebase to answer a question,
- modifying a codebase to pass some criteria,
- browsing a codebase to raise a question.

Some people may not think of the third as a task,
but that's what the author does in most of the spare time,
and somehow doing this actually intrigued the author to further improve the code.

Whether a task can be smoothly done highly depends on the context.
An experienced programmer can quickly find out proper context to explore.
The efficiency of most work in the world is influenced by the quality of context mostly
rather than by the worker's IQ.

litar provides a mechanism to shrink the context to fit a task's needs.
The selection of blocks is based on a concept which is called functional unit.
Let's define the related terms first:

- a functional unit is a file, a chunk reference, or an item;
- a construction block of a functional unit is a block that can be visited by
  traveling through the *C* edges from the applied blocks or defining blocks of a functional unit.

Traditional programming only treats an item or a file as a functional unit,
and the examination or test is usually performed upon the behaviour.
Introducing a chunk reference as a functional unit and focusing on plain text rather than execution
makes another way of examination more convenient.
That is formal verification, which might be adopted extensively in future.

When creating a new node, the node can select a bunch of functional units.
Then all blocks that the new node can visit are selected in three steps:

1. all construction blocks of the functional units;
2. the blocks one step through *R* edges from the blocks of step 1;
3. the blocks one step back through *Ca* edges from the blocks of steps 1 and 2,
   and the blocks one step back through *R* edges from the blocks of step 1.

*R* edges are traced only once.
To know an item exactly, one needs all construction blocks of the block that defines it,
then those of the items they refer to, and so on, which soon covers most of the composition.
To use an item, the block that defines it is enough:
its comment explains the item fully, and the chunks it contains are named after what they do.
A task that needs to know an item exactly selects the item itself.

Stepping back through *Ca* edges brings the blocks that declare the objects quoted in a chunk name.
*Cf* edges are not stepped back, because a shared filter is invoked all over the composition.
Stepping back through *R* edges brings the blocks that use what the task may change.

The selection narrows what a worker reads, not what litar evaluates:
an expression is always evaluated with all blocks of the node,
so building and testing give exact results while the worker reads only the selected blocks.

The node hence has a new value `select`:

```
{
    "description": "a task",
    "ground": "base",
    "nodes": [
        {
            "name": "task",
            "archive": "./task.la",
            "description": "do the task",
            "mappings": [],
            "base": ["Base Node"],
            "select": [
                "Index@/item",
                "File@/file",
                "label@:module@/chunk",
                ...
            ]
        }
    ]
}
```

An entry that does not start with `Index@/` or `File@/` is considered a chunk reference,
an entry that starts with `Index@/` or `File@/` always indicates an item or a file.

# Workflow

By introducing context management,
we can now build a clean workflow for litar-backed development.

An entity to perform a task is called a worker, it can be a human or an agent.

When a worker is given a task, the worker judges the size of the task;
if the task is small, it solves the task by applying blocks, examines the result and ends,
if the task is big, it divides the task into several sub-tasks.
A sub-task can depend on other sub-tasks and starts only after they finish.

The structure of a task is saved in a special chunk `todo list`.
`todo list` does not differ from other chunks for litar,
it only acts as a hint for a worker.

```
@<todo list@=
{
    "name": "<name of the task>",
    "description": "<a short description>",
    "sub-tasks": [
        {
            "name": "<name of sub-task 1>",
            "description": "<a short description>",
            "dependency": []
        },
        {
            "name": "<name of sub-task 2>",
            "description": "<a short description>",
            "dependency": ["<name of sub-task 1>"]
        },
        ...
    ]
}
@
```

After dividing, the worker runs the sub-tasks in order:

1. create a new composition, shrink the context, specify the acceptance,
   for each sub-task that can run in parallel with its siblings;
2. attach candidate workers to the new composition;
3. wait until all the siblings have finished, merge the result:
   merging is not moving blocks, the worker
   treats the results of the sub-tasks as material for developing its own composition;
4. update `todo list`, remove the finished entry and corresponding composition, go to step 1.

Each sub-task can be divided too.
The new compositions are usually placed under the directory of the original composition.
The acceptance is in a chunk that initializes the nodes of a sub-task's composition.
After the task is finished, `todo list` shall be removed from the composition.
