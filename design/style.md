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

This style guidance is for both humans and LLMs to write a well-organized litar archive.
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
`program.c` and `main body` are trees, other chunks are undefined,
but as we see in the following guidelines, we can assume that they are trees from the names.

# Organize a task in a node

Despite the simple linear structure of blocks,
the objects introduced, chunks, modules, items, and files, are highly structured.
Users must deal with the structure carefully to not mess up.

We call a further development based on composed archives a task,
and the lifecycle of a task consists of several stages:

1. write down a brief description of what is going to be done,
   whether it's to develop a new feature, or to improve an existing framework,
   or to add some test cases, etc;
2. find the node this task is based on, create a new node that uses
   that node as its base node;
3. apply new blocks to the new node, until the task is completed;
4. digest the new node, move the newly applied blocks to suitable base nodes,
   if no blocks are left in the new node, delete the new node;
5. test other nodes for integrity.

The digest is to iterate through the newly applied blocks,
apply them to the suitable base node, gather the blocks that remain into a new node,
delete the old node. If no blocks are left, the new node will not be created.

It's context dependent to decide what the suitable base node is.
For example, when using a framework to develop an application,
the framework is built in the base node, the application is in the new node.
Some modification might improve the framework, and the related blocks ought
to be moved to the base node.

# Name a chunk with a phrase, whose grammatical role fits that structure

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
- **omit the quotation if there are many chunks that refer to the same objects**;
- **use concrete verbs, do not use verbs like 'perform', 'do'**.

The quoting here means that if a block of code is meant to add two numbers,
`number1` and `number2`, the result is saved as `number3`,
then the chunk name shall be `save the sum of 'number1' and 'number2' to 'number3'`.

Other structures, such as a complicated data structure or a structured document, etc,
shall be named with a noun phrase, **name a tree with a noun phrase if it's not an action**.

# If a tree is big enough, divide it

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

In practice, the length of a tree usually equals the length of a block.
Here's a more practical guideline: **control the length of a block at or below about 150 tokens**.

# Add a comment before a block that introduces a decision

In the process of implementing an idea, we always make some compromises, trade-offs,
or simply some details the idea has not specified. For example, the size of a buffer,
handling of a failure, choosing of algorithm, etc.

These details live outside the specification, scattered over implementation,
but closely affect the finished program.
litar requires these details to be documented where they are introduced.

# Make blocks extending a sequence order-irrelevant

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

# Reduce repetition by filters

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

