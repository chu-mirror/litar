# Bootstrap implementation details

`src/boot.c` implements `design/litar.md` through Filters,
and `--help` and `-p`/`--print` from `design/ui.md`.
This file records the concrete reading used
where those design files leave a choice open.
`make test` checks this reading.

Names, labels, modules, flags, and label groups
are compared as exact byte strings.
`Light` and `light` are different names.

## Text and controls

A control is `@` followed by a byte below 128
that is outside `A`–`Z` and `a`–`z`.
`@` followed by an ASCII letter, or by any byte 128 or above,
is ordinary text.
There is no escape for a literal control.

An expression ends at `@` followed by space, tab, CR, or LF,
and also at `@` at the end of the buffer.
CR is included so a CRLF archive ends expressions
the same way as a LF archive.

The same rules apply inside block content.
A chunk reference, `@<` … `@>`, may name filters with `@|`.
A `@|` that is not inside a reference ends the body
and is the block's own filter list.
Any other control in content is an error
when that block is expanded.

## Names

A name runs until the next control.
Space, tab, CR, and LF are removed from both ends.
Bytes in the middle stay, including spaces,
so `@- Light @` selects the module `Light`
and `@? included light @` tests the flag `included light`.

An empty chunk name, label, module name, flag, condition,
label-group name, label inside a group, or archive name is an error.

In a qualified name the labels come first,
then one optional module, then the chunk.
A label after `@/`, or a second `@/`, is an error.

The text of `-p` and `--print` is the reference itself.
It ends at the end of the argument,
so a closing `@` is not written on the command line.

## Blocks

`@<name@=` starts a block and `@` followed by whitespace ends it.
The block extends exactly the reference written on that header.
The module is the one named after `@/`,
or the module selected at that point in the file when `@/` is absent.

One leading newline and one trailing newline are layout, not content.
Each of those newlines may be LF or CRLF.
`@<msg@=` followed by a newline, `Hello`, a newline, and `@`
stores `Hello`.
A newline that should remain at either end of the content
takes one extra newline in the file.
Other whitespace is content.

Several blocks may extend one chunk.
Their expanded contents are concatenated
in the order the blocks were parsed.
An included file's blocks appear at its `@.`.

A block with empty content is still a definition.
Asking `-p` for a reference that has no matching block is an error,
`chunk '…' is not defined`.
A reference inside a block that has no matching block
expands to empty text.

## Evaluation

Only the chunk named by `-p` or `--print` is expanded,
and then the references reached from it.
A cycle in some other chunk is left alone.

Expanding a reference that is already being expanded
stops with `circular inclusion of '…'`.
The quoted key is the reference after label groups are expanded:
each label, then `@:`, then `module@/` when the module name is non-empty,
then the chunk name.
The anonymous module contributes no `module@/` piece.

That check runs before anything is written,
so this error leaves stdout empty.

## Modules

The archive starts in the anonymous module, which is the empty name.
`@- name @` selects `name` for later blocks in that file.
`@- @` selects the anonymous module again.
`-p chunk`, with no module in the expression,
asks for the anonymous module,
whichever module was selected last in the file.

A block header may name another module and extend a chunk there.
A reference inside that block that omits the module
still uses the module selected where the block was written.

## Specialization

A block belongs to a reference
when the module and the chunk are the same
and the block's labels are an order-preserving subsequence
of the reference's labels.
Each label of the reference is used at most once in that match.
The block with an empty label list
belongs to every reference of that module and chunk.

`hello` and `executable` both belong to `hello@:executable`.
`executable@:hello` does not.
`a@:a` belongs to a reference that has at least two `a` labels.
Matching blocks are concatenated in parse order,
whether the block is less specialized or is the reference itself.

Label groups are expanded on the reference
and on each block before this comparison.

A reference that writes no labels
keeps the labels of the reference being expanded.
A reference that writes labels uses those labels alone.
This is how `@<includes@>` inside an unspecialized template
receives `program1`
when the template is reached
through `program1@:general structure for C program`.

A reference that writes no module
uses the module selected where the surrounding block was written.
A reference that writes a module uses that module.
A hole written after `@- Light @` stays in `Light`
when another archive references it without naming a module.
A reference written in the anonymous module stays there.

## Label groups

`@:name@=label@:label@` defines `name` for the whole invocation,
including text that came in through `@.`.
The group may be used before or after its definition,
because groups are expanded while evaluating,
after the archive has been read.
Expansion is recursive:
a part that is itself a group is replaced by its parts.
The group name is not also kept as a literal label.

Defining the same group name a second time is an error.
Expanding a group that reaches itself
stops with `circular label group 'name'`.
A chain of 128 groups may expand;
one more stops with `label group 'name' is nested too deeply`.
A group that the requested chunk never expands
is not checked for cycles.

## Branching

`@? condition @|` starts an arm that is kept when that flag is already set.
Further `@?` arms are further conditions.
`@|` starts the arm kept when none of those conditions was kept.
`@` followed by whitespace ends the chain.
The chunk in the design's example is extended by block 1
because `condition A` was set and that arm is first.

An arm ends at the next top-level `@?`, `@|`, or closing `@`.
A `@?` written inside a block is content of that block.
A `@?` after `@|` is `condition after else`.
A second `@|` is `duplicate else`.
The condition is followed by `@|`.
An unclosed chain is `unterminated branch`.

The choice is made while the archive is read, from top to bottom.
`@! name @` sets a flag.
Setting a flag that is already set leaves it set.
A branch sees a flag only when the `@!` was parsed earlier
and its arm was kept.

An arm that is not kept is still parsed.
Syntax there is checked.
The arm does not add blocks, set flags, select a module,
or define a label group,
and an `@.` in that arm does not open a file.

## Including

`@. name @` reads that archive and parses it at that point.
The read is repeated every time the `@.` is reached.
A second include of the same file appends its blocks again,
unless a flag guard skips the body, as in the `c_structure.la` example.
Chunks, flags, and label groups of the included file
stay after the include returns,
which is what makes the guard work on a later include.

The included archive starts in the anonymous module.
When it returns, the including file continues
in the module it had before the `@.`.
A `@-` inside the included file
applies to the blocks that follow it in that file.
A nested `@.` starts in the anonymous module again,
and when that nested include returns
the outer file is back in the module it had selected.

A name that begins with `/` is opened as that path.
Any other name, including one with slashes such as `lib/light.la`,
is looked up in the directories from `design/ui.md`,
joined onto each directory.
The current directory is the process's working directory.

`$LITAR_INCLUDE` is a colon-separated list, in the `$PATH` form.
An empty entry and a trailing colon mean the current directory.
An unset variable and an empty value contribute no directories.
An unset or empty `$HOME`
skips `$HOME/.litar` and `$HOME/.local/share/litar`.

The first regular file wins, following a symbolic link.
A directory of the same name is skipped.
If that file cannot be opened, later directories are not tried.
A missing archive is `could not find archive 'name'`.

An unguarded include cycle is not rejected by remembering the path.
The include stack may hold 64 archives, counting the main file.
One more stops with `includes are nested too deeply in 'path'`
and writes nothing.
A guarded mutual include stays within the limit
because each file's flag is set before it includes the other,
and the second visit takes the empty arm.

## Command line

`--help` in the arguments prints the usage to stdout and exits 0.
It is recognized before the other arguments are checked,
so `--help` with a missing expression or a missing archive
still prints the usage.

The print option is `-p EXPR`, `--print EXPR`, or `--print=EXPR`.
The archive argument may come before or after the option.
One archive is accepted.
A single `-` is an archive name;
any other argument that begins with `-` is an unknown option.

A duplicate print option, an unknown option, a missing expression,
a missing archive, and an extra argument are errors.
Unknown options and missing arguments also print the usage to stderr.
The usage names the program as it was invoked.
The expression form in the usage
includes the filter syntax from `design/ui.md`.

The archive is read to the end before the expression is evaluated.
The chunk is written to stdout as raw bytes, with no added newline.

Opening a file that cannot be opened
reports `could not open 'path':` followed by the system message.
A file that can be opened and then cannot be read,
including a directory on this system,
reports `could not read 'path'`.

## Diagnostics

Errors go to stderr, start with `Error: `, and exit 1.

A parse error adds `at line N in 'path'`.
The line is one plus the number of LF bytes
before the point where parsing stopped.
For the main archive, `path` is the argument that was passed.
For an included archive,
`path` is the path the search returned, such as `./lib.la`.

An error while expanding a block
adds `in chunk '…' (defined at line N in 'path')`,
with the same key spelling as a circular inclusion.
An error in the print expression adds `in expression`.

Parse errors, an unknown top-level chunk, a cycle,
and a bad reference leave stdout empty.
`could not write output` is reported
if stdout cannot be written while the chunk is being emitted.

## Filters

A filter is a chunk.
`@|` names one, and `@|a@|b` is a pipeline:
the text goes through `a`, then through `b`.
On a reference the filters follow the chunk name,
`@<chunk@|filter@>`,
and the same spelling is accepted in `-p` and `--print`.
On a block they follow the body:

```
@<chunk@=
body
@|filter@
```

`@|` in a block header, before `@=`,
is `unexpected control '@|' in name`.
`@|` in a label group is `unexpected control in label group`.
`@|` as an expression of its own is still `unexpected '@|'`.
An empty filter name is `empty filter name`.

The reference is expanded first,
including every reference in a block body.
Each of the block's filters then runs on that text,
and the filter output is the text the block contributes.
A reference's filters run on the expanded chunk.
Filter output is not scanned for further references.
Blocks of one chunk are still concatenated in parse order;
each block is filtered on its own before that concatenation.

A filter with no labels keeps the labels
of the reference being expanded.
Labels written on the chunk reference next to the filter
do not become the filter's labels.
A filter with no module uses the module selected
where the block that names the filter was written.
On a print expression there is no such block,
so a filter with no module is in the anonymous module.
A filter that names a module or labels uses those.

A missing filter is `filter '…' is not defined`,
with the same key spelling as a chunk.
The program that filter expands to
must begin with `#!`.
Otherwise the error is
`filter '…' does not start with a shebang`.
The interpreter and one optional word are taken from that line,
the way the kernel loads a script,
and the interpreter is started with the program in a temporary file.
The filter's standard input is the text being transformed.
Its standard output replaces that text.
Its standard error is this process's standard error.
The working directory is the process's current directory.
A non-zero exit is `filter '…' exited with status N`.
A signal is `filter '…' exited with signal N`.
A failure to start the interpreter is
`could not run filter '…':` followed by the system message.

The whole result is held until every filter has finished,
then written to stdout.
A filter therefore runs once,
and a failed filter or a cycle leaves stdout empty.
Expanding the filter program is an ordinary chunk expansion,
so a filter that reaches the chunk already being expanded
stops with `circular inclusion of '…'`.

`Text Processing@/path to perl interpreter`
in `lib/text-processing.la` is the shell script `which perl`
run by `Basics@/evaluate`.
Its result is the path printed by `which perl`.

## Later stages

File definitions and file sets are recognized and rejected.

- `@[` and `@(`: `file definitions are not implemented`
- `` @` ``, `@+`, and `@,`: `file sets are not implemented`
- any other control used as an expression: `unknown expression '@c'`
