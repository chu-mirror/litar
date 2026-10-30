# Implementation

This is the plan for a Go program that implements
[design/litar.md](../design/litar.md),
[design/ui.md](../design/ui.md),
and [syntax.md](syntax.md).
The binary is `litar`.
It reads a composer file, not a positional archive.

The C program in `src/litar.c` is a reference for byte-level scan details
and for error tone.
It is not the spec.
Do not copy its `.` rule or its label rule.
It resolves `.` against the reference being expanded,
and it drops inherited labels when the block already carries
a proper suffix of them.
The design resolves `.` against the block header,
and an omitted label list is the full list of the reference being expanded.
The Go program does not parse `@.`, `@?`, label groups,
or a positional `ARCHIVE` argument.

# Layout

The module lives at the repository root.
The module path is `litar`.
The language is Go 1.24 or later.

```
cmd/litar/main.go
internal/litar/
    scan.go
    name.go
    parse.go
    compose.go
    eval.go
    list.go
    filter.go
    fuse.go
    cli.go
    mcp.go
go.mod
```

`cmd/litar` only parses `os.Args` and calls `litar.Main`.
All behaviour lives in `internal/litar`,
one package, files split by concern.
`fuse.go` and `mcp.go` may move to subpackages
when their dependencies would otherwise weigh on unit tests.

The Makefile builds `./litar` with `go build -o litar ./cmd/litar`.
`make test` runs `go test ./...` and then `tests/run.sh`.
`src/litar.c` stays until the Go binary covers the stages below;
then the C target is dropped.

Standard library first.
`github.com/hanwen/go-fuse/v2` for the mount.
The official MCP Go SDK if it speaks protocol `2026-07-28`;
otherwise a small JSON-RPC stdio server for that protocol.
`gofmt` on every file.
User errors go to stderr, exit 1, and leave stdout empty.

# Types

A name is a decoded, trimmed string.
Interior whitespace stays.

```
type Name string

type ModulePath []Name // empty is the root module

type ChunkRef struct {
    Labels []Name
    Module ModulePath
    Chunk  Name // empty only on an invocation with a pipeline
}

type Filter struct {
    Ref  ChunkRef
    View []ViewOp
}

type ViewOp struct {
    Kind ViewOpKind // AddSet, SubSet, AddFile, DelFile
    Name Name
}

type Item []Name

type Op byte // '=' or '+'

type Part struct {
    Text string
    Ref  *Ref // set when this part is a chunk reference
}

type Ref struct {
    ChunkRef
    Filters []Filter
}

type Block struct {
    Ref      ChunkRef
    Op       Op
    Parts    []Part
    Filters  []Filter
    Items    []Item
    Comment  string // bytes after the previous expression
    Raw      string // the expression as written, for -l
    Source   string // archive path this block was parsed from
    StartLine, EndLine int
    // Set when the expression is @[ or @(.
    // The block is still in the node's block sequence.
    // Ref is the root module and the chunk name is Path.
    // A file has no labels.
    IsFile     bool
    Path       Name
    Executable bool
    Sets       []Name
}

type Node struct {
    Name        string
    Archive     string // resolved path
    ArchiveSpec string // as written in the composer
    Description string
    Mappings    []Mapping
    Base        string // empty for the root
    Own         []Block // this node's archive only, in order
}

type Mapping struct {
    Path    string
    FileSet []string
    Origin  string // resolved, relative to the composer file
}

type World struct {
    Composer string
    Nodes    []Node // introduction order, tree via Base
}
```

`Chain(node)` is the base node's chain followed by `Own`.
Evaluation and `-l` use the chain.
Delete, dump, and `blocks://node` use `Own`.
Two nodes share an archive when their resolved paths are equal.

# Scan

Implement [syntax.md](syntax.md) Scan and Names as a byte cursor.

`@` plus an ASCII letter, or `@` plus a byte at or above 128,
is text.
`@` plus any other ASCII byte is a two-byte control.
`@@` is one `@` and does not start or end an expression.
An end control is `@` plus space, tab, CR, or LF,
or a final `@` at end of input.
`@` CR LF consumes `@` CR as the end control;
the LF stays outside the expression.

A name runs from the current position to the next control.
Trim space, tab, CR, and LF at both ends,
then decode `@@`.
An empty name after trimming is an empty name.

The scanner reports the control kind, the name, and the byte offset.
Parse builds on that.
Do not scan with regular expressions.

# Parse

Top-level forms are `@<` a block and `@[` or `@(` a file.
Any other top-level control is an error.

A block header is a qualified name, then `@=` or `@+`.
`@|` in the header is an error.
The first CR, LF, or CR LF after the operator is layout.
The body is text, `@@`, and references `@<` … `@>`.
`@|` at body depth 0 starts the filter pipeline.
`@~` at body depth 0 starts items, after the pipeline if there is one.
The closing end control is required.

A reference in the body may carry a pipeline and a view per filter.
Each `@<` raises depth and each `@>` lowers it.
`@@` does not change depth.
An end control still ends the block.

A file path is one name.
A slash in it is ordinary.
`` @` `` plus a non-empty file-set name may repeat after the path.
Then `@=` or `@+` and a body like a block.

A command-line name uses the same qualified-name parser
with the argument as the whole input and no surrounding `@<` `@>`.
`-p` allows a pipeline and views.
`-l`, `-e`, and `-d` do not.

Parse errors name the file, the line, and the unexpected control.

# Composer

`-f` names a JSON array of nodes.
Each node has `name`, `archive`, `description`, `mappings`, and `base`.
`base` is another node name, or `null` for the root.
Require exactly one root, unique names,
an acyclic base graph,
every node reachable from the root,
and distinct archive paths.

If `archive` starts with `.` or `..`,
resolve it against the composer file's directory.
Otherwise look for that name as a file in:

1. the process current directory
2. directories in `$LITAR_INCLUDE`, `$PATH` form
3. `$HOME/.litar`
4. `$HOME/.local/share/litar`
5. `/usr/local/share/litar`
6. `/usr/share/litar`

The first regular file wins.

A mapping's `origin` is relative to the composer file.
`path` is the path in the filter mount.
`fileset` is a list of file-set names.
A child keeps its base's mappings and may add more.
File-set membership is the `Sets` on file blocks in the chain,
plus the mappings inherited along the same chain.
`@+name` adds every file and mapping that lists `name`.

Parse each archive once into that node's `Own`.
`-n` selects the node whose chain is evaluated.
If `-n` is omitted, use the root.

# Evaluation

A block matches a chunk reference when the module and chunk are equal
and the block's labels are a suffix of the reference's labels.
Empty labels match every specialization.
The predecessors of `spec2@:spec1@:module@/chunk`
are `spec1@:module@/chunk` and `module@/chunk`.

Apply matching blocks in node order.
`@=` replaces the text accumulated so far.
`@+` appends.
The blocks that still contribute are the effective blocks
of that chunk reference.

For each applied block, expand its parts,
run its filter pipeline on that text,
then set or append.
Until stage 6, a pipeline is an evaluation error,
`filters are not implemented`,
so a filter is not dropped quietly.
Expanding a reference uses the labels of the chunk reference
currently being expanded when the inner reference omits labels.
Labels written on the inner reference are that reference's labels.
An omitted module is the root.
`.` and `..` resolve against the module of the chunk reference
the surrounding block is applied to,
which is the module on that block's header.
`.` and `..` are an error in a header or a filter name.

A missing inner reference expands to empty text.
A missing top-level `-p`, `-l`, or `-e` reference is an error.
Re-expanding a chunk reference already on the visit stack is a cycle:
stderr names the chain, stdout is empty, exit 1.

Filter output is finished text and is not scanned again.

A filter name is resolved like a body reference for labels.
Its omitted module is the root.
A filter cannot use `.` or `..`.

# Listing

`-l` prints, for each matching block including predecessors,
the comment before the block and `Raw`.
`-e` uses the same form on the effective blocks.
`-d` lists blocks whose content contains, or whose pipeline names,
the query or a reference specialized from the query.
A use of `label@:chunk` depends on `chunk` and on `label@:chunk`.
`-r` lists blocks that carry that exact item.

`-m MODULE` lists modules under that module:
full paths that extend `MODULE`,
joined by `@/`, in first-introduction order.
`-m /` lists every module except the root.
The root is written `/`.
`-c MODULE` lists chunk names whose module equals `MODULE`,
one name per line, first-introduction order.
A chunk in a descendant module is not included.

Each `@~` path is one item.
The root domain is `/`, the empty prefix.
`-s DOMAIN` lists the name that follows `DOMAIN`
in any item path, once each, in first-seen order.
The line is that domain plus the name, joined by `@/`.
`-s /` lists the first name of each item path.
`-i DOMAIN` lists every item path
whose prefix before the last name is `DOMAIN`,
joined by `@/`.
`-i /` lists item paths of one name.
`-r` matches an item path exactly.

# Filters

A filter chunk evaluates to a script whose first line is a shebang.
The shebang names the interpreter and an optional argument.
Write the script to a host temporary file.
Stdin is the text being transformed.
Stdout replaces that text.
Stderr of the process is litar's stderr.
A non-zero status fails the evaluation.

Just before `exec`, compute the view from the empty set
with `@+`, `@-`, `@,`, `@!` left to right.
Mount a FUSE filesystem that contains those files and no others.
Archive files and mapped files are read-only.
The process cwd is the mount.
Absolute host paths such as `/bin/sh` remain reachable.
Child processes inherit the same mount.

Writes go to a temporary overlay on the host,
visible through the mount, so the process can create a file and read it back.
The archive is unchanged.
The overlay is discarded and the mount is removed when the process exits.

A file's content is the evaluated root-module chunk named by its path.
Mapped files are the bytes of `origin`.
Content is produced when the process reads the file,
on the same visit stack as the chunk being expanded.
A read that re-enters a chunk reference already on that stack
fails the invocation and names the chain.
The view is not evaluated up front.

`github.com/hanwen/go-fuse/v2` implements the mount.
Tests that need a mount skip when `/dev/fuse` is missing.
Stage 6 does not mount. See Stages.

# Command line

```
usage: litar [options]
```

Help text matches [design/ui.md](../design/ui.md).
The usage line writes `argv[0]` where that file writes `litar`.
`--help` is long only.
Short and long options, and `--name=value`, are all accepted.
One action per invocation.
Combining two of `-p -l -e -d -m -c -s -i -r -a` is an error.
The message names the earlier mode in the order above.

`-f` is required for every action except `--help` and `-a`.
`-a` may take `-f`; if it does not,
the MCP tool that specifies the composer supplies it.

PROGRAM in the usage line is `argv[0]`.

# MCP

`-a` serves protocol
[2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28)
on stdio.
The composer path is process state, not a protocol session.
Until a composer is specified,
the only tool is the one that sets the composer,
and resource lists are empty.
That tool creates a JSON array `[ ]` if the file does not exist,
then writes a root node on the first `create node`.

Resources and tools are those in [design/ui.md](../design/ui.md).
Percent-encode `@`, space, and slash in URI components.

`path://composer` and `path://node/{node}` return a host path as text.
`file://{node}/{path}` returns the evaluated bytes of that file.
`tree://` returns the JSON object with `name`, `description`, and `sub-trees`.
Node descriptions come from the composer.
Module, chunk, and item descriptions are empty
until reserved-namespace lookup is specified without clashing with `@/`.

`blocks://` returns text in the `-l` form.
`blocks://node/{node}` is every block in that node's own archive.
`blocks://apply`, `blocks://effective`, and `blocks://depend`
are the `-l` form of `-l`, `-e`, and `-d` on that node.
`blocks://refer` is the `-l` form of `-r`.

Tool arguments:

- specify the composer: `path`
- create a node: `name`, `archive`, `description`, `base`
  (`base` empty creates the root; the composer must have no root yet)
- delete a node: `name`
- delete a chunk: `node`, `chunk reference`
- delete a file: `node`, `path`
- evaluate an expression: `node`, `expression`
- apply a block: `node`, `text`
- dump the archives: no arguments

`apply a block` appends `text` to the node's archive
only when `ArchiveSpec` starts with `.` or `..`.
`delete a chunk` and `delete a file` remove matching blocks
from that node's own archive, not from the base.
`delete a node` removes the node and every node based on it
from the composer and does not delete archive files.
`dump` rewrites each node's own archive:
drop a block that is not an effective block
of its own chunk reference in that same node;
keep the comment and `Raw` of the blocks that remain.

Digest, from [design/style.md](../design/style.md), is not a tool.
It is apply on the base, override or delete on the child, dump,
then delete the child node.

After a mutation, notify through `subscriptions/listen`
with `resourcesListChanged` and `resourceSubscriptions`
as the protocol requires.

# Tests

`internal/litar` has table tests for scan, names, parse,
suffix matching, label inheritance, `.` / `..`,
effective-block sequences, and depend/item queries.

CLI tests stay in `tests/` with `run.sh`.
A case is a directory with `driver.sh` or `argv`.
`argv` is one argument per line and includes `-f` and the composer.
`run.sh` no longer treats a bare `expr` as
`litar -p EXPR archive.la`.
Do not keep the C cases for `@.`, `@?`, or label groups
as acceptance tests for the Go binary.

Stage tests as the stages land:
blocks and newlines, modules, specialization, composer chains,
listings, filters with empty views, then FUSE views.

# Stages

Finish each stage, including its tests, before the next.

## 1. Scan and names

Cursor, controls, `@@`, end control, trimmed names,
qualified names, command-line names.
No archive yet.

## 2. Parse one archive

Blocks, files, pipelines, views, items, comments, `Raw`.
Reject nested top-level controls and unterminated blocks.
`go test` on golden snippets from [syntax.md](syntax.md).

## 3. Composer and node chain

JSON, search path, mappings inheritance,
concatenate blocks along `base`.
Errors for two roots, a cycle, a missing archive,
and two nodes sharing an archive.

## 4. Evaluate without filters

Match, apply, expand, inherit labels, resolve `.` and `..`,
cycles, missing inner vs missing top-level.
`-p` on chunks that have no pipeline.
A pipeline is the error `filters are not implemented`.

## 5. Listings

`-l -e -d -m -c -s -i -r`,
literal form, CLI combination errors, `--help`.
`run.sh` takes `argv` with `-f`.

## 6. Filters without a mount

Shebang, stdin, stdout, non-zero status.
The script is a host temporary file.
cwd is a temporary directory.
The view is checked and then ignored:
no archive file is visible.
There is no FUSE mount in this stage.

## 7. Files, views, and FUSE

File sets, lazy views, read-only files, overlay writes
that the process can read back,
mapped files, cycle reports when a file read re-enters the visit stack.
Tests that need a mount skip when `/dev/fuse` is missing.

## 8. MCP

Stdio server, the tool arguments above, resources, apply, delete, dump,
notifications.

The libraries under `lib/` and the examples under `examples/`
still use include and branch syntax.
They are rewritten after stage 5, in the new language,
as a separate archive task.
They are not a requirement for the Go binary to parse them
in the old form.
