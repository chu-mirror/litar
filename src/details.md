# Bootstrap implementation details

`src/boot.c` implements `design/litar.md` through Filters,
and `--help`, `-p`/`--print`, `-l`/`--literal`, `-m`/`--modules`,
and `-d`/`--depend`
from `design/ui.md`.
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
`@@` is the text `@`.
It does not start an expression, end a name, or open a reference.
Each `@` that would otherwise be a control is written twice,
so `@@<kept@@>` is the text `@<kept@>`,
and the name `a@@b` is the name `a@b`.

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

The text of `-p`, `--print`, `-l`, `--literal`, `-d`, and `--depend`
is the reference itself.
It ends at the end of the argument,
so a closing `@` is not written on the command line.
`-l`, `--literal`, `-d`, and `--depend` name a chunk reference,
so `@|` there is `unexpected control '@|' in name`.

## Blocks

`@<name@=` and `@<name@+` start a block,
and `@` followed by whitespace ends it.
`@=` sets the chunk reference to the expanded content of that block.
`@+` extends the chunk reference with that content.
A block header that is neither is `expected '@=' or '@+'`.
The block names exactly the reference written on that header.
The module is the one named after `@/`,
or the module selected at that point in the file when `@/` is absent.

One leading newline is layout, not content.
That newline may be LF or CRLF.
It is layout after `@=` and after `@+`.
`@<msg@=` followed by a newline, `Hello`, a newline, and `@`
stores `Hello` and the final newline.
A newline that should remain at the start of the content
takes one extra newline in the file.
The newline before the closing `@` or `@|` is content.
Other whitespace is content.

Matching blocks are applied in the order they were parsed.
`@=` replaces the text accumulated so far,
including text from earlier unspecialized predecessors.
`@+` appends the block to that text.
A later `@=` overrides earlier blocks of the same reference.
The first matching block is applied to empty text,
so `@=` and `@+` agree when a reference has one matching block.
An included file's blocks appear at its `@.`.
A block that a later `@=` replaces is still expanded.
Its references are followed and its filters run,
so a cycle or a filter failure there still stops evaluation.

A block with empty content is still a definition.
Asking `-p` for a reference that has no matching block is an error,
`chunk '…' is not defined`.
A reference inside a block that has no matching block
expands to empty text.

## Evaluation

Only the chunk named by `-p` or `--print` is expanded,
and then the references reached from it.
`-l` and `--literal` do not expand any chunk.
`-m` and `--modules` do not expand any chunk.
`-d` and `--depend` do not expand any chunk.
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
and the block's labels are a suffix
of the reference's labels.
Labels are dropped from the left
to walk back along one specialization path.
`spec2@:spec1@:chunk` therefore contains
the blocks of `chunk`,
the blocks of `spec1@:chunk`,
and the blocks that extend `spec2@:spec1@:chunk`.
It does not contain the blocks of `spec2@:chunk`.
`spec2@:chunk` specializes `chunk` on another path,
and `spec1@:chunk` does not contain those blocks either.

The block with an empty label list
is the unspecialized chunk
and belongs to every reference of that module and chunk.

`executable` belongs to `hello@:executable`.
`hello` does not,
and `executable@:hello` does not.
`a@:a` belongs to a reference
whose labels end in `a@:a`.
`a` belongs to `a@:a`,
because that single label is a suffix.
Matching blocks are applied in parse order.
`@=` replaces the text accumulated so far.
`@+` appends to it.
A block is included
whether it names a predecessor on that path
or the reference itself.

Label groups are expanded on the reference
and on each block before this comparison.

A reference that writes no labels
keeps the labels of the reference being expanded.
A reference that writes labels uses those labels alone.
When the block that contains the reference
already carries a proper suffix of those labels,
the reference keeps only the labels in front of that suffix.
It does not repeat the labels the block already has.
An unspecialized block carries none,
so the reference keeps every label.
This is how `@<includes@>` inside an unspecialized template
receives `program1`
when the template is reached
through `program1@:general structure for C program`.
A block extending `spec1@:chunk`,
reached through `spec2@:spec1@:chunk`,
passes only `spec2` to an unlabeled reference.

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
so `--help` with a missing expression, a missing chunk reference,
or a missing archive still prints the usage.

The synopsis is `usage: PROGRAM [options] ARCHIVE`.
PROGRAM is the name used to invoke litar.
The option lines follow `design/ui.md`,
and the usage also spells the forms of `EXPRESSION` and `CHUNK_REFERENCE`.

The print option is `-p EXPR`, `--print EXPR`, or `--print=EXPR`.
The literal option is `-l REF`, `--literal REF`, or `--literal=REF`.
The modules option is `-m` or `--modules`.
It takes no argument.
`--modules=…` is an unknown option.
The depend option is `-d REF`, `--depend REF`, or `--depend=REF`.
Print, literal, modules, and depend cannot be combined.
The archive argument may come before or after the option.
One archive is accepted.
A single `-` is an archive name;
any other argument that begins with `-` is an unknown option.

A duplicate print option, a duplicate literal option,
a duplicate modules option, a duplicate depend option,
print combined with literal, print combined with modules,
print combined with depend, literal combined with modules,
literal combined with depend, modules combined with depend,
an unknown option, a missing expression, a missing chunk reference,
a missing archive, and an extra argument are errors.
With no action selected the error is
`missing -p, --print, -l, --literal, -m, --modules, -d, or --depend`.
The combined-option errors are
`cannot combine print and literal`,
`cannot combine print and modules`,
`cannot combine print and depend`,
`cannot combine literal and modules`,
`cannot combine literal and depend`,
and `cannot combine modules and depend`.
A repeated modules option is `duplicate modules option`.
A repeated depend option is `duplicate depend option`.
Unknown options and missing arguments also print the usage to stderr.
The expression form in the usage
includes the filter syntax from `design/ui.md`.

The archive is read to the end before the expression is evaluated,
before literal blocks are printed,
before the blocks that depend on a reference are printed,
and before the module list is written.
A print, literal, or depend result is written to stdout as raw bytes,
with no added newline.

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
An error in the print expression,
or in the chunk reference passed to `-l` or `-d`,
adds `in expression`.

Parse errors, an unknown top-level chunk, a cycle,
and a bad reference leave stdout empty.
`could not write output` is reported
if stdout cannot be written while the chunk is being emitted.

## Literal blocks

`-l` and `--literal` print the blocks that extend the chunk reference,
in the same order `-p` applies them.
The same blocks are chosen as for `-p`:
the module and the chunk match,
and the block's labels are a suffix
of the reference's labels.
Label groups in the reference and on each block
are expanded before that test.
A reference with no matching block is `chunk '…' is not defined`.

Each matching block is printed in archive order.
The text is the comment immediately before that block,
then the requested chunk reference,
then the content of the block,
and the filters applied to the block.
The comment is the text after the previous expression
in that file, or from the start of the file,
up to the block.
Inside a branch arm it starts after the `@|` that opens the arm.
An expression between two blocks is not part of either comment,
so it is left out.
`@-` is not printed.
A comment that is only spaces, tabs, and newlines
is left out when it would be the first text printed,
so the result does not open with the spacing that followed a `@-`.
A later blank comment is printed,
so a blank line between two printed blocks stays.
A comment that contains other text is printed whole,
including the blank lines around that text.
The printed chunk reference names the labels of the request,
then the module the block extends, then the chunk.
Label groups in the request are expanded before they are written,
so `pair@:base` is printed as `lab@:extra@:base`
when `pair` stands for `lab@:extra`.
The labels are not omitted when this block's header left them out.
The module name is not omitted when the block extends a named module,
even if the header in the file left the module out.
A header that already names a module keeps that module:
the block extends the named module,
not the module selected around the header.
The anonymous module has an empty name,
so its reference has no `module@/` piece.
`@` in a label, a module name, or a chunk name is written as `@@`.

The bytes after `@=` or `@+` are the layout newline, the content,
the block's filters, and the closing `@`.
The printed header uses that block's operator,
so an extending block is printed with `@+`.
A block whose labels are exactly the requested labels
is copied as written.
A block with fewer labels is printed as the requested specialization.
Each chunk reference in its content that names no label
receives only the labels the request adds
in front of that block's own labels.
An unspecialized block receives every requested label.
A block that already carries some of them
does not copy those labels onto the reference.
A reference that already names labels is kept as written,
including one whose labels are only some of the requested labels.
The module written on an included reference is not filled in.
`@@` stays `@@` and does not become a reference.
The block's own `@|` filters are copied as written
and do not receive the labels.
References are not followed, block filters are not run,
and filter output is not scanned,
so a cycle that `-p` rejects is ordinary text for `-l`.
A malformed reference in a body is not an error for `-l`
and is copied as written.
Included blocks are printed from the included file.
The pieces are concatenated in parse order,
with nothing added between them.

A `-l` reference that names no module
matches that chunk in every module.
`-p` still means the anonymous module when the expression names none.
A `-l` reference that names a module matches only that module.
Less specialized blocks are those of the same module and chunk
whose labels are a suffix of the reference.
An included file starts in the anonymous module.
Its blocks do not show the includer's `@-`.

This is the example in `design/ui.md`:

```
@- module 1 @

@<chunk@=
@<substructure 1@>
@

@<chunk2@=
@<substructure@>
@

@- module 2 @

Extend chunk through label.
@<label@:module 1@/chunk@=
@<substructure 2@>
@
```

`litar -l label@:module 1@/chunk` prints

```
@<label@:module 1@/chunk@=
@<label@:substructure 1@>
@

Extend chunk through label.
@<label@:module 1@/chunk@=
@<substructure 2@>
@
```

The reference names `module 1`,
so the block under `module 2` is included
only because its header names `module 1`.
The unspecialized block is printed
because the empty label list is a suffix of `label`.
Its header gains `label` and `module 1`,
and `@<substructure 1@>` gains `label`
because that reference names no label.
`chunk2` extends a different chunk, so it is left out.
`Extend chunk through label.`
is the comment before the specialized block.
The blank line before that comment is part of the comment.
The blank line after `@- module 1 @` is a blank comment
on the first block, so it is left out.
The specialized block already has the requested labels,
so `@<substructure 2@>` stays as written.
Both printed references name `label` and `module 1`.

## Blocks that depend

`-d` and `--depend` list every block that depends on the chunk reference,
in archive order.
A block depends on the reference
when a direct use in that block names it.
A use is a chunk reference in the content,
a filter on such a reference,
or a filter on the block itself.
Text in the comment is not a use.
The search is not transitive:
a block that names some other chunk
does not depend on the chunks that other chunk names.
Chunks are not expanded and filters are not run,
so a missing filter is not an error
and a cycle is ordinary text.
The reference does not have to be defined.
When no block depends on it,
stdout is empty and the exit status is 0.
A block that names itself is listed.

Each use is resolved
the way `-p` resolves it
when the block is expanded
at the specialization that block extends.
A use that writes labels uses those labels.
A use that writes none takes the block's own labels.
An unspecialized block therefore sees an unlabeled use
as the unspecialized chunk,
not as a specialization passed in from outside.
A use that writes a module uses that module.
A use that writes none uses the module selected
where the block was written.
That can differ from the module named on the block header.
Label groups on the query, on the block, and on the use
are expanded before the comparison.
The use matches the query
when the query is that use
or the use is a chunk reference specialized from the query.
The query's labels are a suffix of the use's labels,
the chunks are the same,
and the modules agree.
`label@:chunk` matches the query `chunk`
and the query `label@:chunk`.
`spec2@:spec1@:chunk` matches `spec1@:chunk` and `chunk`.
It does not match `spec2@:chunk`,
which is another specialization path.
A use of `chunk` does not match the query `label@:chunk`.
A query that names no module matches every module.
A query that names a module matches only that module.

This is the example in `design/ui.md`:

```
@<chunk 1@=
content
@

@<chunk 2@=
@<label 1@:chunk 1@>
@

@<chunk 3@=
content
@|label 2@:chunk 2@
```

`litar -d 'chunk 1'` and `litar -d 'label 1@:chunk 1'` print the second block.
`@<label 1@:chunk 1@>` is `label 1@:chunk 1`,
and that reference is specialized from `chunk 1`.
The first block extends `chunk 1` and does not use it.
`litar -d 'chunk 2'` and `litar -d 'label 2@:chunk 2'` print the third block,
because its content is filtered by `label 2@:chunk 2`.
`litar -d 'label 1@:chunk 2'` prints nothing.

A reference in the content that is not well formed
does not match and is not reported.
Scanning continues after it,
so a later well-formed reference in the same block still counts.
An arm that is not kept contributes no block.

The output has the same form as `-l`.
Each matching block is the comment immediately before it,
then the chunk reference that block extends,
then the content and the filters as they were written.
The block's labels are written as stored.
A label that names a group is not expanded in that header.
A named module is written
even when the header left the module out.
The anonymous module has an empty name,
so its reference has no `module@/` piece.
`@` in a printed label, module, or chunk is written as `@@`.
The body is not rewritten as a further specialization,
so labels are not added to references inside it.
A blank comment is left out
when it would be the first text printed.
A later blank comment is printed.
Nothing is added between blocks.

On the example in the literal section,
`litar -d 'module 1@/substructure 1'` prints the unspecialized block
with the module filled in,
and does not add `label` to its header or its body.
The block under `label` does not depend on that reference.
Its unlabeled `@<substructure 2@>` is `label@:substructure 2`
in `module 2`,
the module selected where that block was written.
That use is specialized from `substructure 2`,
so `litar -d 'substructure 2'` prints the block.
`litar -d 'module 1@/substructure 2'` prints nothing,
because the use is in `module 2`,
not in `module 1`, the module the block extends.

## Summary chunks

`summary` is an ordinary chunk.
The bootstrap does not create one, and it does not create a `meta` chunk.
A module may define `summary` the same way as any other chunk.
`-p` evaluates that chunk and `-l` prints it.
A module with no `summary` block is `chunk '…' is not defined`
when that chunk is requested.

## Listing modules

`-m` and `--modules` print every module the archive introduces,
one name per line, in the order it was introduced.
Each line ends with LF, including the last.
The anonymous module is introduced first.
Its name is empty, so its line is empty.
`@- name @` introduces `name` when that arm is kept.
A kept block whose header names a module introduces that module.
A module that is only named by a reference is not introduced.
An arm that is not kept does not introduce a module.
An included archive introduces modules while it is read.
Selecting or extending a module a second time does not list it again.

`-m` does not expand chunks and does not run filters.
A parse error is reported and nothing is listed.

## Filters

A filter is a chunk.
`@|` names one, and `@|a@|b` is a pipeline:
the text goes through `a`, then through `b`.
On a reference the filters follow the chunk name,
`@<chunk@|filter@>`,
and the same spelling is accepted in `-p` and `--print`.
`-l` and `-d` reject `@|`,
because a chunk reference has no filters.
On a block they follow the body:

```
@<chunk@=
body
@|filter@
```

`@|` in a block header, before `@=` or `@+`,
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
Blocks of one chunk are still applied in parse order.
Each block is filtered on its own,
then `@=` sets the accumulated text to that result
and `@+` appends it.

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
- `` @` ``, `@+`, and `@,` used as an expression: `file sets are not implemented`
  `@<name@+` is a block that extends a chunk, not a file set.
- any other control used as an expression: `unknown expression '@c'`
