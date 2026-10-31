# Syntax

This document defines the syntax of a litar archive.
It is the scan of the text and the form of every expression.
[design/litar.md](../design/litar.md) introduces the same language.
A qualified name on the command line uses the form in the last section.
[design/ui.md](../design/ui.md) defines the options around that name.

An archive is a sequence of bytes.
The scan reads one byte at a time.
A byte at or above 128 is ordinary text.

# Scan

An `@` begins a control when the next byte is an ASCII non-letter.
The ASCII letters are `A` through `Z` and `a` through `z`.
The control is those two bytes.
There is no space between them.

`@@` is an escape.
It is one ordinary `@`,
and it neither starts nor ends an expression.
A name stores that single `@`.

An `@` followed by an ASCII letter,
or by a byte at or above 128,
is ordinary text.
Both bytes stay as they are.
`@word` needs no escape.
An `@` that would be a control is written twice,
so the text `@<kept@>` is written `@@<kept@@>`.

An expression ends at an end control.
The end control is `@` followed by one byte of space, tab,
carriage return, or line feed,
or a final `@` at the end of the input.
That `@` and the one whitespace byte are outside the name and the content.
`@` followed by a carriage return consumes those two bytes.
A line feed after that carriage return stays outside the expression.

Any other `@` plus a non-letter starts a nested control.
A control that no form accepts is an error.

These are the controls:

| Control | Form |
| --- | --- |
| `@@` | one ordinary `@` |
| `@` at a space, tab, carriage return, line feed, or end of input | end the current expression |
| `@<` | block header, or a reference in a body |
| `@>` | end a reference |
| `@=` | set the chunk |
| `@+` | extend the chunk, or add a file set |
| `@-` | subtract a file set |
| `@/` | separate two module components, separate the module from the chunk, or separate two names of an item |
| `@~` | start an item reference on a block |
| `@:` | separate labels |
| <code>@&#124;</code> | filter or end of a block body |
| `@!` | remove one file |
| `@[` | regular file |
| `@(` | executable file |
| `` @` `` | file-set membership on a file |
| `@,` | one file in a file-set expression |

At the start of an expression the control selects the form:
`@<` a block,
`@[` or `@(` a file.
A top-level `@|`, a top-level `@~`, a bare end control, or any other control is an error.

# Names

A name is the bytes from the current position up to the next control.
`@@` is part of the name and becomes one `@`.
Space, tab, carriage return, and line feed are removed from both ends
after the name is taken and before `@@` is decoded.
Interior whitespace stays, including a newline between two words.
`@<Module 1@/chunk@=` names the chunk `chunk` in the module `Module 1`.

A name that is empty after trimming is an empty name.
An empty chunk, label, module component, or filter name is an error.

# Qualified name

A qualified name is zero or more labels,
then a module path,
then a chunk:

```
label@:label@:module@/submodule@/chunk
```

Each label is a name followed by `@:`.
The leftmost label is the further specialization.
Every later name separated by `@/` is one component of the module path;
the last name is the chunk.
The module path may be omitted, which names the root module.
A leading `@/` names the root module explicitly.
Labels come first, then the module path, then the chunk.
A `@:` after the module path has started is an error.
An empty component is an error, except for the leading `@/`.

The module paths form a tree.
A module is a directory and a chunk is a file in that tree.
The parent of every module is the root module, written `/`.
The components of a path are its directories below the root.
The root module can usually be omitted.

In the content of a block, a module path that starts with `.` or `..`
is resolved against the module of the chunk reference the block is applied to.
`.` names that module, and `..` names its parent.
`..` at the root stays at the root.
A module path in a block header is absolute: a `.` or `..` component
there is an error.

The chunk name is required in a block header,
on the command line, and in a filter.
A reference may omit it only in the invocation form
in the file-set section.

# Specialization

A chunk reference is a qualified name.
Applying a block through one chunk reference
does not affect other chunk references of the same chunk.

The predecessors of `spec2@:spec1@:module@/chunk`
are `spec1@:module@/chunk` and `module@/chunk`.
`spec2@:spec1@:module@/chunk` does not include blocks applied to
`spec2@:module@/chunk`.
That reference follows a different path to specialize `module@/chunk`.

A block matches a chunk reference when the module and the chunk are the same
and the block's labels are a suffix of the reference's labels.
Empty labels match every specialization.
Matching blocks are applied in the order they appear in the node:
the base node's blocks, then the blocks of this node's archive.
`@=` replaces the text accumulated so far.
`@+` appends.
The blocks that still contribute after this
are the effective blocks of the chunk reference.

When a reference in a body omits labels,
it takes the labels of the chunk reference being expanded.
Labels written on the inner reference are the labels of that reference.
A reference that omits `@/` names the root module.
Only a reference in block content resolves `.` and `..`.

# Comments and expressions

Bytes that are outside an expression are a comment.
A comment is ordinary text under the same scan.
`@<` in a comment starts an expression.
The way to write that text is `@@<`.

A block or file that omits `@/` belongs to the root module.

# Block

A block sets or extends a chunk:

```
@<qualified@=
content
@
```

`@=` sets the chunk to this block.
`@+` extends it.
The qualified name has no filter pipeline.
`@|` in the header is an error.

The body follows the operator.
A carriage return, a line feed, or a carriage return and a line feed
immediately after `@=` or `@+` is layout, not content.
At most one carriage return and one line feed are removed,
and the carriage return is removed only when it comes first.
`@<msg@=` followed by a line feed, `Hello`, a line feed, and `@`
stores `Hello` and the second line feed.
The line break before the closing end control is content.
A one-line body stores no added line break:

```
@<msg@=Hello@
```

The closing `@` is required.
End of input without it is an unterminated block.
End of input immediately after that `@` is a valid end control.

`@|` at the top of the body ends the content and starts a filter pipeline.
The pipeline is still inside the block,
and an end control closes the block:

```
@<chunk@=
content
@|filter@
```

`@~` at the top of the body ends the content and starts an item reference.
Items follow the filter pipeline when the block has one.
The item is not part of the content.
Evaluating the block does not print it.

```
@<chunk@=
int times;
@|filter
@~ C@/variables@/times @
```

An item is one or more names separated by `@/`.
The first name is the domain.
Each later name classifies that item further.
There is no fixed depth.
`@~ C@/variables@/times @` names `C`, then `variables`, then `times`.
A second `@~` starts another item on the same block:

```
@~C@/variables@/times@~C@/functions@/main@
```

The block's end control closes the last item.
An empty name in the path is an error.
`@@~` in the content is the text `@~` and does not open an item.
A `@~` inside a chunk reference stays in the body.

A reference in the body is `@<`, a qualified name, and `@>`.
`@|` between that `@<` and its `@>` belongs to the reference
and stays in the body.
Each `@<` increases the reference depth and each `@>` decreases it.
`@@` does not change the depth.
An end control still ends the block at any depth.

The body contains text, escapes, and references.
Any other control in the body is an error.
`@@<` is the text `@<` and does not open a reference.
`@~` at the top of the body is the item list above, not content.

# Filter pipeline

A filter is a qualified name with no pipeline of its own:
optional labels, an optional module, and a chunk name.
`@|` separates filters in a pipeline, left to right.
`@|a@|b` is two filters.
An empty filter name is an error.

In a reference or a print expression the pipeline follows the chunk:

```
@<chunk@|one@|two@>
```

On a block or a file the pipeline follows the body, as above.
A chunk reference used with `-l`, `-e`, or `-d` has no pipeline.

The filter is a chunk.
Its first line is a shebang.
Each filter may have a view, in the file-set section.

# File

`@[` introduces a regular file.
`@(` introduces an executable file.
The path is one name, not a qualified name.
A slash in the path is an ordinary byte.
`@/` is still a control and does not appear in a path.

Zero or more file-set memberships follow the path.
Each membership is `` @` `` and a non-empty file-set name.
`@=` follows, and the body is the body of a block,
including a filter pipeline after the content.
`@+` is not paired with `@[` or `@(` .
A file has no labels and no module.

```
@[hello.c@`source files@`executables@=
content
@
```

```
@(hello.sh@=
content
@
```

The path is the chunk name.
The chunk belongs to the root module.
Each membership names a file set that contains this file.

# File set

A file-set expression is a list of operations, read left to right,
starting from the empty set of files.

| Operation | Operand | Effect |
| --- | --- | --- |
| `@+` | file-set name | add every file in that set |
| `@-` | file-set name | remove every file in that set |
| `@,` | file path | add that file |
| `@!` | file path | remove that file |

Each operand is a name.
`@+file set1@-file set2@,file1@,file2@!file3`
is every file in `file set1` that is not in `file set2`,
plus `file1` and `file2`,
except `file3`.

A view is a file-set expression attached to a filter,
after that filter's chunk name and before `@|`, `@>`,
the end control of the block, or the end of a print expression:

```
@<@|list files@+source files@>
```

This reference omits the chunk name.
The omitted chunk is the empty input of the pipeline.
The empty chunk name is allowed only on a reference
that has a pipeline.
The view belongs to the filter it follows.
Each filter in a pipeline has its own view.
By default an invocation sees no file exported from the archive.

# Reserved namespaces

Four module names hold meta chunks:
`Module`, `Chunk`, `Domain`, and `Index`.
The rest of the module path, followed by the chunk name,
is the path of the object the chunk describes.

```
Module@/Basic
Module@/Text Processing@/Internals
Chunk@/Basic@/evaluate
Domain@/C@/variables
Index@/C@/variables@/times
```

`Module@/Basic` describes the module `Basic`.
`Chunk@/Basic@/evaluate` describes the chunk `evaluate` in that module.
`Chunk@/name` describes a chunk in the root module.
`Domain@/C@/variables` describes the domain `C@/variables`.
`Index@/C@/variables@/times` describes the item `times` in that domain.

The block evaluates to a JSON object:

```
{"description": "<description of that module, chunk, domain, or item>"}
```

The chunk tree, `-m`, and `-c` do not list these chunks.
The description is shown on the module, chunk, domain, or item.
`-m`, `-c`, `-s`, and `-i` print the name.
When the description is not empty, the line is `Name:`,
then tabs, then the description.
The tabs put every description in that list at the same column.

# Composer file

The command line reads the composer file in a directory,
not a single archive.
The `-t`/`--target` option names that directory.
litar reads `composer.json` inside it.
[design/litar.md](../design/litar.md) defines its format:
a JSON object with `ground` and `nodes`.
`ground` is the name of a composed archive this one is based on,
or `null`.
Each node has a `name`, an `archive`, a `description`,
`mappings`, and a `base`.
`base` is the name of another node, or `null` for the root node.
A composed archive is a directory that contains `composer.json`.
litar searches the ground in this order:

1. the current directory;
2. directories in `$LITAR_LIBRARY`, in the same format as `$PATH`;
3. `$HOME/.litar`;
4. `$HOME/.local/share/litar`;
5. `/usr/local/share/litar`;
6. `/usr/share/litar`.

The first directory `NAME/composer.json` that is a regular file wins.
Every node in the ground precedes the nodes in this file.
The combined nodes have one root.
Node names are unique.
Two nodes cannot share one archive.
An archive name is relative to the composer file that names it.

A node's blocks are its base node's blocks
followed by the blocks of its archive.
The `-n`/`--node` option selects the node that the command line evaluates at.

The `mappings` field maps files outside the archives
into the filesystem used by an invocation.
Each mapping has a `path`, a `fileset` array of file-set names,
and an `origin`.
`origin` is relative to the composer file.
The mapped file is read-only.
A child keeps its base's mappings and can add more.

# Command-line name

`-p` takes a qualified name and an optional pipeline,
with no surrounding `@<` and `@>`,
and with no end control:

```
label1@:label2@:module name@/chunk name@|filter1@|filter2
```

The name runs to the end of the argument.
Each filter may have a view, as in the file-set section.
`-l`, `-e`, and `-d` take the same qualified name without a pipeline.
A module that is omitted on any of them names the root module.
There is no option that matches a chunk in every module.

`-l` prints every matching block, including blocks applied to predecessors.
`-e` prints the effective blocks of that chunk reference.
`-d` lists blocks that depend on the chunk reference:
a block whose content contains, or is filtered by,
that reference or a reference specialized from it.

`-m`/`--modules` and `-c`/`--chunks` take a module path,
not a chunk reference:

```
module@/submodule@/subsubmodule
```

The root module is `/`.
`-m` lists all modules under that module.
`-c` lists all chunk names under that module.
The output of `-m` uses `@/` between components,
so it can be passed back to an option that takes a module path.
The output of `-c` is one chunk name per line.

`-s` takes a domain and lists all subdomains under that domain.
`-i` takes a domain and lists all items under that domain.
A domain is a prefix of an item:

```
domain@/subdomain
domain@/subdomain@/item
```

The root domain is `/`.
The output of `-s` and `-i` uses `@/` between names.

`-r` takes an item, not a chunk reference.
The item is one or more names separated by `@/`,
with no `@~` and no closing `@`:

```
domain@/subdomain@/item
```

A block refers to that item when one of the items on the block is that item.
The listing of `-e`, `-d`, and `-r` has the same form as `-l`.
