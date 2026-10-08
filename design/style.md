# Introduction

litar roots in Donald Knuth's
[Literate Programming](http://www.literateprogramming.com/knuthweb.pdf).
Knuth says that to write a program as an essay; a program written
in this form will have better quality and be more understandable.
Unfortunately, when it comes to AI era,
most of the authoring work is done by LLMs rather than by human.
LLMs robbed the pleasure of writing from human beings, sigh.

Anyway, we must adapt to the new paradigm for our salary.
Literate Programming shines once again, if it have ever shined,
when the cooperation between human and LLMs became more and more important.

The problem is that LLMs produce output too quick to examine all the details,
but we still want a way to grasp what LLMs have done.
To command an agent to explain itself is one method;
litar tries to provide a more efficient way for this purpose:
let LLMs write programs in literate program form
to make the output more accessible.

This style guidance is for both human and LLMs to write a well organized litar archive.
The language used to constrain the authoring of a litar archive
is based on the discussion of structure's shapes.
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
and there are two shapes of structure found here: sequence and tree.

A structure of sequence shape is formed by a series of substructures of same type,
for example, a series of include files, a series of functions.
A structure of tree shape is formed by a fixed composition of substructures.

Among the chunks above, `includes`, `variables`, and `functions` are sequence,
`program.c` and `main body` are tree, other chunks are undefined,
but as we see in the following guidelines, we can assume that they are tree from the names.

# If a tree is big enough, divide it

So how to determine a tree is big or not?

Imagine that you are facing a thousands lines C source file without a header file,
and you are trying to figure out what this file tells.
A skilled engineer will never read this file line by line.
The steps he follows might be,

1. go straight to functions, find a global function and start reading it;
2. find the data types that the outermost logic processes;
3. list all global functions touching the same data types;
4. figure out what this group functions do.

The C source file can be regarded as a structure of tree shape,
its substructures are functions, data types; a global function is also a tree,
it has an outermost logic, the details are encapsulated in substructures.

The actions the skilled engineer takes are searching for functions,
skimming the outermost logic of a function, searching for data types,
and searching for functions have references to a particular data type.

A main object of litar is to make the actions based on the structures easier,
and the first step is to make the structures explicit.
So an acceptable answer to the question at the opening is that the tree is lengthy
enough so that to take actions costs several minutes for a skilled engineer.
The number of tokens digested by LLMs is a good index for length;
150 tokens is a comfortable length for human beings, at least for the author.

To put several substructures in a sequence to a single block does not differ much from
to divide them to different blocks, so in practice, the length of a tree usually equals the length of a block.
Here's a more practical style guiding: **control the length of a block around 150 tokens**.

# Name a chunk as a phrase, its grammatical function makes sense

First of all, because the name of a chunk is a phrase,
**do not capitalize the first character and do not append a period at the end**.

Naming a sequence is simple; a sequence is always a list of items,
so **name a sequence as a plural noun phrase**.

Naming a tree is much more difficult; it heavily depends on context.

# Add comment before a block when an assumption was made in that block

# Make blocks extending a sequence order-irreverent

# Avoid repetition by filters

# Format files by filters

