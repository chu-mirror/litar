# Introduction

litar roots in Donald Knuth's
[Literate Programming](http://www.literateprogramming.com/knuthweb.pdf).
Knuth says that to write a program as an essay; a program written
in this form will be better and more understandable.
Unfortunately, when it comes to AI era,
most of the authoring work is done by LLMs rather than by human.
LLMs robbed the pleasure of writing from human beings, sigh.

Anyway, we must adapt to the new paradigm for our salary.
Literate Programming shines once again, if it have ever shined,
when the cooperation between human and LLMs became more and more important.

The problem is that LLMs produce output too quick to examine all the details,
but we still want a way to know what LLMs have done.
To command an agent to explain itself is one method;
litar tries to provide a more efficient way for this purpose:
let LLMs write programs in literate program form
to make use of understandability advantages of Literate Programming.

This style guidance is for both human and LLMs to write a well organized litar archive.
The language used to constrain authoring of a litar archive
is based on discussion of structure's shapes.
The knowledge of human for something is always structurized, so is for programs.
For example, a C source file, from the outermost,
consists of includes, macros, variables, and functions, etc.
A C source file hence shows itself in a litar archive typically as following:

```
@[program.c@=
@<includes@>
@<variables@>
@<functions@>
@

@<functions@=
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

It's clear that what chunks encapsulate is structures,
and there are two shapes found here for structures: sequence and tree.

A structure of sequence shape is formed by a series of items of same type,
for example, a series of include files, a series of functions.
A structure of tree shape is formed by a fixed composition of substructures.
Among the chunks above, `includes`, `variables`, and `functions` are sequence,
`program.c` and `main body` are tree, other chunks are undefined,
but as we see in the following guidelines, we can assume that they are tree from the names.

# If a tree is big enough, divide it


