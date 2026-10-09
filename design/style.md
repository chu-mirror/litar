litar is rooted in Donald Knuth's
[Literate Programming](http://www.literateprogramming.com/knuthweb.pdf).
Knuth says to write a program as an essay; a program written
in this form will be better and more understandable.
Unfortunately, in the AI era,
most of the authoring work is done by LLMs rather than by humans.
LLMs robbed human beings of the pleasure of writing, sigh.

Anyway, we must adapt to the new paradigm.
Literate Programming shines once again, if it has ever shone,
when the cooperation between humans and LLMs became more and more important.

The problem is that LLMs produce output too quick to examine all the details,
but we still want a way to grasp what LLMs have done.
To command an agent to explain itself is one of the methods;
litar tries to provide a more efficient way for this purpose:
let LLMs write programs in literate program form to make the output more accessible.

This style guidance is for both human and LLMs to write a well-organized litar archive.
The language used to constrain the authoring of a litar archive
is based on the discussion of structure's shapes.

The knowledge of human for something is always structured, so is for programs.
For example, a C source file, at the outermost level,
consists of includes, macros, variables, and functions, etc.
A C source file hence shows itself in a litar archive typically as follows:

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

It's clear that a chunk encapsulates a structure,
and there are two shapes of structure found here: sequence and tree.

A structure of sequence shape is formed by a series of substructures of the same type,
for example, a series of include files, a series of functions.
A structure of tree shape is formed by a fixed composition of substructures.

Among the chunks above, `includes`, `variables`, and `functions` are sequences,
`program.c` and `main body` are tree, other chunks are undefined,
but as we see in the following guidelines, we can assume that they are trees from the names.

# If a tree is big enough, divide it

So how do we determine whether a tree is big enough?

Imagine that you are facing a C source file of thousands of lines without a header file,
and you are trying to figure out what this file does.
A skilled engineer will never read this file line by line.
The steps he follows might be,

1. go straight to functions, find a global function and start reading it;
2. find the data types that the outermost logic deals with;
3. list all global functions touching the same data types;
4. figure out what this group functions does.

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
Tokens count is a good measure of length;
150 tokens is a comfortable length for human beings, at least for the author.

Putting several items of a sequence to one block is much like giving them seperate blocks;
so in practice, the length of a tree usually equals the length of a block.
Here's a more practical guideline: **control the length of a block at or below about 150 tokens**.

# Name a chunk with a phrase, whose grammatical role fits

First of all, because the name of a chunk is a phrase,
**do not capitalize the first character and do not append a period at the end**.
And one principle is mandatory:
**name a chunk with a short description of the encapsulated structure**.
It might be unnecessary to stress this principle, but anyway, the principle is the most important.

Naming a sequence is simple; a sequence is always a list of items,
so **name a sequence with a plural noun phrase**.

Naming a tree is much more difficult; it depends heavily on the context.
When programming in imperative style, the expressions usually operate on something,
so a verb phrase is appropriate for encapsulating these expressions.
When programming in functional style, the expressions are simply values,
so a noun phrase is more suitable here.
Even these apparent rules are not always applicable; like the `main body` in the example above,
it's named from a declarative viewpoint.

Nevertheless, there are some advice for more concrete situations.
For imperative expressions, the rules are well established:

- **quote the objects to be operated in the phrase**;
- **omit the quotation if there are many chunks refer to the same objects**;
- **use concrete verbs, do not use verbs like 'perform', 'do'**;
- **name a tree from a declarative view point if no verb fits**.

The quotations here means that if a block of code is meant to plus two numbers,
`number1` and `number2`, the result is saved as `number3`,
then the chunk name shall be `plus 'number1' and 'number2', save the result to 'number3'`.

Other structures, such as a complicated data structure or a structured document, etc,
shall be named with a noun phrase, **name a tree with a noun phrase if it's not an action**.

# Add comment before a block that introduce a decision

In the process of implementing an idea, we always make some compromises, trade-offs,
or simply some details the idea has not specified. For example, the size of a buffer,
handling of a failure, choosing of algorithm, etc.

These details live outside the specification, scattered over implementation,
but closely affect the finished program.
litar requires these details to documented where they are introduced.

# Make blocks extending a sequence order-irrelerent

A data type definition consists of other data types;
so a data type should be put after the data types it's based on.
In this case, if the data type and the base data types are in different blocks,
the dependency imposes an order on blocks.

The blocks shall be loosely related in the sense of ordering
to reduce the context needed for understanding or editing a block.
The `includes` in the above example is an ideal order-irrelerent sequence;
if a block is created to introduce more header files,
the only context information is the name of the chunk it ought to extend, `includes` here.
Also, readers do not have to list all the blocks of `includes` to figure out what the current block
is supposed to do.

Sometimes, the ordering is unavoidable. For example, in an if-else chain, once one arm matches,
every later arm is skipped even if its condition would be true.

The rule: **gather substructures of a sequence into a single block if the order matters**.
This rule might conflict with the rule of block length limitation,
if they conflict, prioritize this rule.

