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
A nested block or branch consumes its own end control.
A control that no form accepts is an error.

These are the controls:

| Control | Form |
| --- | --- |
| `@@` | one ordinary `@` |
| `@` at a space, tab, carriage return, line feed, or end of input | end the current expression |
| `@<` | block header, or a reference in a body |
| `@>` | end a reference |
| `@=` | set the chunk, or bind a label group |
| `@+` | extend the chunk, or add a file set |
| `@-` | subtract a file set |
| `@/` | separate two module components, separate the module from the chunk, or separate two names of a topic |
| `@~` | start a topic reference on a block |
| `@:` | separate labels, or start a label group |
| <code>@&#124;</code> | filter, branch arm, or end of a block body |
| `@!` | declare a flag, or remove one file |
| `@?` | branch |
| `@.` | include an archive |
| `@[` | regular file |
| `@(` | executable file |
| `` @` `` | file set |
| `@,` | one file in a file-set expression |

At the start of an expression the control selects the form:
`@<` a block,
`@!` a flag,
`@?` a branch,
`@:` a label group,
`@.` an include,
`@[` or `@(` a file,
`` @` `` a file-set declaration.
A top-level `@+` or `@,` is a file-set control.
A top-level `@|`, a top-level `@~`, a bare end control, or any other control is an error.

# Names

A name is the bytes from the current position up to the next control.
`@@` is part of the name and becomes one `@`.
Space, tab, carriage return, and line feed are removed from both ends
after the name is taken and before `@@` is decoded.
Interior whitespace stays, including a newline between two words.
`@<Module 1@/chunk@=` names the chunk `chunk` in the module `Module 1`.

A name that is empty after trimming is an empty name.
An empty chunk, label, module component, flag, label-group, filter,
or archive name is an error.

# Qualified name

A qualified name is zero or more labels,
then a module path,
then a chunk:

```
label@:label@:module@/submodule@/chunk
```

Each label is a name followed by `@:`.
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

A label may name a label group.
The group stands for its sequence of labels.
The sequence is expanded at the use.

# Comments and expressions

Bytes that are outside an expression are a comment.
A comment is ordinary text under the same scan.
`@<` in a comment starts an expression.
The way to write that text is `@@<`.

An included archive is parsed at the `@.` expression.
Every block in it names its own module, so inclusion needs no module state.
Included chunks, flags, label groups, and modules remain.

A block or file that omits `@/` belongs to the root module.
A reference that omits `@/` names the root module.
Only a reference in block content resolves `.` and `..`.

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

`@~` at the top of the body ends the content and starts a topic reference.
Topics follow the filter pipeline when the block has one.
The topic is not part of the content.
Evaluating the block does not print it.

```
@<chunk@=
int times;
@|filter
@~ C@/variables@/times @
```

A topic is one or more names separated by `@/`.
The first name is the domain.
Each later name classifies that topic further.
There is no fixed depth.
`@~ C@/variables@/times @` names `C`, then `variables`, then `times`.
A second `@~` starts another topic on the same block:

```
@~C@/variables@/times@~C@/functions@/main@
```

The block's end control closes the last topic.
An empty name in the path is an error.
`@@~` in the content is the text `@~` and does not open a topic.
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
`@~` at the top of the body is the topic list above, not content.

# Flag and include

A flag is `@!`, a non-empty name, and an end control.

An include is `@.`, a non-empty archive name, and an end control:

```
@. c_structure.la @
```

# Branch

A branch is one or more conditions, an optional else arm,
and an end control:

```
@? condition A @|
expressions
@? condition B @|
expressions
@|
expressions
@
```

Each condition is `@?`, a non-empty name, and `@|`.
The else arm is a `@|` that is not attached to a condition.
Expressions in the arm follow that `@|`.
An arm ends at the next `@?`, the next unattached `@|`,
or the branch's end control.
Text in an arm is a comment, or part of an expression in that arm.

The kept arm is the first condition whose flag is declared.
The else arm is kept when none of those flags is declared.
A skipped arm contributes no blocks, modules, flags,
label groups, includes, or files.

The first condition comes first.
Later conditions follow it.
The else arm, when present, is last.
A condition after the else arm is an error.
A second else arm is an error.
An arm contains no branch:
an inner `@?` ends the arm.
The include guard is this form with an empty first arm:

```
@? included common C structure @|@|
```

# Label group

A label group binds a name to one or more labels:

```
@:spec@=spec2@:spec1@
```

The form is `@:`, a non-empty group name, `@=`,
then one or more non-empty labels separated by `@:`,
then an end control.
A newline after `@=` is leading whitespace of the first label
and is trimmed.

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
A chunk reference used with `-l` or `-d` has no pipeline.

# File

`@[` introduces a regular file.
`@(` introduces an executable file.
The path is one name, not a qualified name.
A slash in the path is an ordinary byte.
`@/` is still a control and does not appear in a path.

Zero or more file-set memberships follow the path.
Each membership is `` @` `` and a non-empty file-set name.
`@=` or `@+` follows, and the body is the body of a block,
including a filter pipeline after the content:

```
@[hello.c@`source files@`executables@=
content
@
```

```
@(run_bigger_world.sh@=
content
@
```

The path is the chunk name.
The chunk belongs to the root module.
Each membership names a file set that contains this file.

The program recognizes `@[` and `@(`
at the start of an expression and rejects the expression
before reading the path.
The form above is the syntax those controls introduce.

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

A declaration is `` @` ``, a non-empty file-set name,
a file-set expression, and an end control:

```
@`file set@+file set1@-file set2@,file1@,file2@!file3@
```

A view is a file-set expression attached to a filter,
after that filter's chunk name and before `@|`, `@>`,
or the end of a print expression:

```
@<@|list files@+source files@>
```

This reference omits the chunk name.
The omitted chunk is the empty input of the pipeline.
The empty chunk name is allowed only on a reference
that has a pipeline.
The view belongs to the filter it follows.
Each filter in a pipeline has its own view.

The program recognizes `` @` ``, `@+`, and `@,`
at the start of an expression and rejects the expression.
It also rejects an empty chunk name,
and it rejects `@+`, `@-`, `@,`, and `@!` inside a filter name.
Those controls are the file-set syntax in this section.

# Command-line name

`-p` takes a qualified name and an optional pipeline,
with no surrounding `@<` and `@>`,
and with no end control:

```
label1@:label2@:module name@/chunk name@|filter1@|filter2
```

The name runs to the end of the argument.
`-l`, `-e`, and `-d` take the same qualified name without a pipeline.
A module that is omitted on any of them names the root module.
There is no option that matches a chunk in every module.

`-m` and `-c` take a module path, not a chunk reference:

```
module@/submodule@/subsubmodule
```

The root module is `/`.
`-m` lists the immediate child modules of that module.
`-c` lists the chunks defined in that module itself.
The output of -m uses `@/` between components,
so it can be passed back to an option that takes a module path.
The output of -c is one chunk name per line.

`-s` takes a domain and lists its immediate subdomains.
`-i` takes a domain and lists its immediate items.
A domain is a prefix of a topic path, and an item ends one:

```
domain@/subdomain
domain@/subdomain@/item
```

The output of -s and -i uses `@/` between names.

`-r` takes an item, not a chunk reference.
The item is one or more names separated by `@/`,
with no `@~` and no closing `@`:

```
domain@/subdomain@/item
```

A block refers to that topic when one of its topic paths is the topic
or continues it with further names.
`C` matches a path `C` and a path `C@/variables@/times`.
`C@/variables@/times` does not match a block that names only `C`.
`C@/var` does not match `C@/variables`.
The listing has the same form as `-l`.
