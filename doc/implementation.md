# Implementation

This document describes the Go implementation of `litar`: its package layout, data model,
parser, composer loader, evaluator, filter execution, command-line interface, and MCP server.
See [syntax.md](syntax.md) for the archive language and `composer.json` specification.

# Layout

The Go module `litar` (Go 1.25, formatted with `gofmt`) lives under `src/` and is generated
from the composition in `litar/`:

- `cmd/litar/main.go`: entry point that passes `os.Args` to `litar.Main`.
- `cmd/litar-web/main.go`: web UI for browsing and editing that spawns `litar -a` and serves its MCP
  resources over HTTP.
- `internal/litar/`: single package containing the implementation, split by concern:
  - `types.go`: core data structures and path/reference helpers
  - `parse.go`: archive and command-line expression parser
  - `compose.go`: `composer.json` loader, node graph, and archive compaction
  - `eval.go`: chunk reference evaluation and cycle detection
  - `list.go`: block and tree listings (`-l`, `-e`, `-d`, `-r`, modules, chunks, items)
  - `filter.go`: file catalog, view evaluation, tangling, and filter execution
  - `fuse.go`: FUSE filesystem for filter views
  - `cli.go`: command-line flag parsing and dispatch
  - `mcp.go`: MCP stdio server, resources, tools, and metadata evaluation
  - `rewrite.go`: renaming chunks, modules, items, and domains, the `rm` and `mv` of files, mappings,
    and the override, deletion, and move of single blocks

External dependencies are limited to `github.com/hanwen/go-fuse/v2` (for filter view mounts)
and `github.com/modelcontextprotocol/go-sdk` (for the MCP server).

At the repository root, the `Makefile` evaluates the node `Litar Build` (`litar/build.la`),
based on the node `Litar`; it maps `bin/` and `src/` of the repository into
the filter views: `make` runs `go build` in a view of the go module that writes the binaries
into `bin/`, `make sync` writes the generated files, except the Go test files, into `src/`,
`make mod` runs `go mod edit` and `go mod tidy` on `litar/mod/go.mod` and `litar/mod/go.sum`,
which the go command manages outside the archives and the node `Litar` maps at `mod/`,
and `make test` runs `go test ./...` in a view of the go module with the file set `test data`,
which the node `Litar` maps from `examples/` and `standard/` into `internal/litar/testdata/`,
then the acceptance cases in views of `bin/` and their fixtures.

Each litar process keeps every file it evaluates, together with a digest of each chunk
and filter view the evaluation used; the file is evaluated again only when one of them
changes, so a file is evaluated once per process and the MCP server serves repeated reads
from memory. A mapping whose path ends in `/` maps a host directory into the views of its
file sets; the directory is created if missing, and filters read and write it in place.
On any user error, `litar` prints the error to `stderr`, leaves `stdout` empty, and exits
with status 1.

# Data

The core types in `types.go` are designed around lossless source round-tripping and node
inheritance:

- **`Block`:** Stores `Comment` (the raw comment bytes preceding the expression) and `Raw`
  (the exact expression bytes as written, including its closing end control). This allows
  `-l` and archive compaction to reproduce source blocks byte for byte. `Trailing` on `Node`
  and `Archive` preserves any trailing bytes after the final block.
- **`EndsLine`:** Boolean on `Block` recording whether the block's content ended with a
  layout line break (`\n` or `\r\n`), used when joining `@+` blocks.
- **Files as blocks:** A file expression (`@[` or `@(`) is stored as a `Block` with
  `IsFile = true`; its `Ref` is a chunk in the root module named after the file's `Path`.
- **`Own` vs. chain:** A node's `Own` slice holds the blocks parsed from that node's archive
  file. Its **chain** is the concatenation of all blocks along its `base` ancestry (from the
  root node down) followed by `Own`. Evaluation and listings use the full chain; edits,
  compaction, and `blocks://node` operate only on `Own`.

# Parse

`parse.go` scans bytes directly with a cursor over the controls defined in
[syntax.md](syntax.md), without regular expressions:

- **Layout line breaks:** At most one `\r\n`, `\n`, or `\r` immediately after `@=` or `@+`
  is consumed as layout. At most one `\r\n` or `\n` at the end of the body content—before
  `@`, `@|`, or `@~`—is stripped as layout and sets `EndsLine = true`.
- **Body and inner references:** In a block body, `@<` invokes `parseInnerRef`, which reads
  a qualified name and an optional `@|` filter pipeline up to `@>`. At the top level of the
  body (outside `@<...@>`), `@|` starts the block's filter pipeline, `@~` starts the item
  list, and an end control closes the block. Reaching an end control or EOF inside an
  unclosed `@<...` reference fails with `unterminated block`.
- **Command-line expressions:** Command-line arguments use the same qualified-name parser
  over the entire argument string. `-p` allows a filter pipeline and views; `-l`, `-e`, and
  `-d` reject pipelines.
- **Error reporting:** Archive parse errors include the file and line number
  (`<msg> at line <N> in '<file>'`), while command-line parse errors report
  `<msg> in expression`. Invalid file headers report `files are paired only with @=`,
  `files cannot be specialized`, or `files cannot be modularized`.

# Composer

`compose.go` loads compositions and manages their node graphs (see [syntax.md](syntax.md)
for the `composer.json` schema and ground search path):

- **Ground loading:** If `ground` is non-null, the ground composition is located on the
  search path and loaded first (recursively). Its nodes precede the current file's nodes.
  Cycles across grounds fail with `cycle in grounds: a -> b -> a`.
- **Graph validation:** The combined node list must have a single root (`base: []`). Every
  non-root node's `base` must name an ancestor path already present in the list
  (`base '…' of node '…' is not defined`), sibling names under the same parent must be
  unique, and no two nodes may share an archive file.
- **Archive parsing:** Each node's archive is parsed once into `Own`; a missing archive
  fails the load.
- **File catalog:** A node's file-set catalog combines the `Sets` declared on file blocks
  in its chain with the `Mappings` inherited along its `base` chain. In a view expression,
  `@+name` adds every archive file and mapped file belonging to `name`. Defining the same
  path as both an archive file and a mapping is reported as an error when that file is used.
- **Locating an archive (`-f`):** `FindArchive` walks upward from the archive's directory
  looking for `composer.json`. The nearest `composer.json` that declares the archive as one
  of its own local nodes wins. If none matches, it returns the first composer load error
  encountered, or `no composition declares archive '…'`.

# Evaluation

`eval.go` expands chunk references on a node's block chain:

- **Matching:** A block matches a chunk reference when their module and chunk name are equal
  and the block's labels are a suffix of the reference's labels.
- **Accumulation:** Matching blocks are applied in chain order:
  - `@=` replaces the accumulated text with the block's expanded text;
  - `@+` appends the block's expanded text, joined by `\n` when the block has `EndsLine` and
    the accumulated text is non-empty.
  The matching blocks that still contribute to the final text (from the last `@=` onward)
  are the **effective blocks**.
- **Block expansion:** Each applied block expands its body parts in order (running any
  reference-level filter pipeline on each inner reference), runs the block's own filter
  pipeline on the combined text, and then sets or appends. Filter output is final and is
  not scanned for controls.
- **Indentation:** While expanding a block, `continueLine` follows the text of the current
  line of the block's own parts. When that text is non-empty and holds only spaces and tabs
  at an inner reference, `indentLines` writes it before every line after the first of the
  reference's text (after the reference-level pipeline), skipping empty lines and the end
  of the text. Text included by an earlier reference on the same line makes the line
  non-blank, whatever it expanded to.
- **Resolution and inheritance:**
  - An inner reference without labels inherits the labels of the chunk reference currently
    being expanded.
  - An omitted module is the root module. In a body reference, `.` and `..` resolve relative
    to the enclosing block's header module (they are rejected in block headers, filter names,
    and CLI arguments).
  - A filter name resolves like a body reference (inheriting labels from the reference or
    block it is attached to), except that `.` and `..` are not allowed.
- **Missing references and cycles:**
  - A missing inner reference (or a missing archive file chunk during tangling) expands to
    empty text `""`.
  - A missing top-level reference (`-p`, `-l`, `-e`) or a missing filter chunk fails with
    `chunk '…' is not defined`.
  - Re-expanding a chunk reference that is already on the active visit stack fails with
    `circular reference a -> b -> a`.

# Listing

`list.go` implements the query flags:

- **`-l` (literal):** Prints `Comment` + `Raw` for every block matching the chunk reference
  (including blocks applied to predecessors), in chain order. Fails if no block matches.
- **`-e` (effective):** Prints `Comment` + `Raw` for the effective blocks of the chunk
  reference. Fails if no block matches.
- **`-d` (depend):** Prints `Comment` + `Raw` for every block in the chain whose body
  references or filter pipelines use the queried chunk reference or a specialization of it.
- **`-r` (reference):** Prints `Comment` + `Raw` for every block in the chain that carries
  the exact queried item.

Module, chunk, subdomain, and item trees are exposed through the MCP server (and tested via
`list.go`). They omit the reserved namespaces (`Chunk`, `Specialization`, `File`, `Index`)
and populate descriptions by evaluating those metadata chunks.

# Filters

`filter.go` and `fuse.go` run filter pipelines in isolated FUSE views:

1. **Shebang and script:** A filter chunk must evaluate to a script whose first line is
   `#! interpreter [arg]`; as with the kernel's script loader, `arg` is the rest of the line,
   passed as one argument without its surrounding blanks (so `#!/usr/bin/env -S cmd args`
   works). The script is written to a temporary file on the host; the input
   text is piped to `stdin`, `stdout` replaces the text, `stderr` passes through to litar's
   `stderr`, and a non-zero exit code fails the evaluation.
2. **View calculation and tangling:** Before the filter process starts, its view operations
   (`@+`, `@-`, `@,`, `@!`) are applied left to right starting from the empty set, and every
   resulting file is tangled:
   - an archive file is evaluated as a root-module chunk on the current visit stack;
   - a mapped file is read from its host `origin` path.
   Any circular dependency or unreadable file aborts the invocation before the process starts.
3. **FUSE mount and overlay:** The tangled files are mounted as a read-only FUSE filesystem
   (`go-fuse/v2`) whose file stats (mode, size, inode, link count, timestamps) are fixed at
   tangle time. Any files or directories created by the filter are written to a temporary
   host overlay directory that is visible through the mount during the invocation and
   discarded afterwards. The filter runs with the mount as its working directory while
   absolute host paths remain reachable.

Three implementation constraints govern the FUSE mount:

- **Entering the mount via `/bin/sh`:** The child process enters the mount via
  `/bin/sh -c 'cd "$0" && exec "$@"'` instead of setting `cmd.Dir`. If `cmd.Dir` were set to
  the FUSE mount, Go's `vfork` child would `chdir` into a filesystem served by the parent Go
  process before `exec`, which can deadlock if the Go runtime initiates a garbage-collection
  stop-the-world.
- **Lazy unmount fallback:** If normal unmounting fails after the filter exits (for example,
  because the filter left a background process running in the mount), the mount is detached
  lazily (`MNT_DETACH` or `fusermount -u -z`) rather than waiting indefinitely.
- **Signal handling:** `SIGINT`, `SIGTERM`, and `SIGHUP` caught by `litar` detach all live
  mounts before terminating.

Tests that require FUSE skip automatically when `/dev/fuse` is absent.

# Command line

```
Usage: litar [-a] [-f ARCHIVE] [-l CHUNK_REFERENCE] [-e CHUNK_REFERENCE] [-d CHUNK_REFERENCE] [-r ITEM] [-p EXPRESSION]

-h, --help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-f, --file ARCHIVE: specify the archive file, find the composition and the node corresponding
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
```

- The command is a command of the Go ground: Go's `flag` package parses the invocation
  syntax, and the usage above is generated from it.
- Each option has a short and a long name, registered as two flags of one variable.
  Either name may follow one or two hyphens, and the value may be the next argument or
  follow `=` (`-f ARCHIVE`, `--file=ARCHIVE`). Attached short values (`-fARCHIVE`) and
  clusters (`-al`) are unknown flags.
- `-h`, `-help`, or `--help` prints the usage on stdout and exits 0, unless it is the value
  of another option. An unknown flag prints the error and the usage on stderr and exits 2;
  a positional argument prints the usage on stderr and exits 1.
- A repeated option keeps its last value.
- `ARCHIVE` is a path to a `.la` archive file; `CHUNK_REFERENCE`, `EXPRESSION`, and `ITEM`
  follow the command-line syntax in [syntax.md](syntax.md).
- An action is given by a non-empty operand. Exactly one action (`-p`, `-l`, `-e`, `-d`, `-r`,
  or `-a`) may be given per invocation.
  Combining two actions fails with `cannot combine <first> and <second>`, naming the earlier
  action in the order `print`, `literal`, `effective`, `depend`, `reference`, `agent`.
- `-f` is required for every action except `-a`.
- `-a -f ARCHIVE` starts the MCP server with `ARCHIVE`'s composition already imported;
  `-a` alone starts the server with no compositions imported.

# MCP

`litar -a` serves MCP protocol version
[2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28) over `stdio`.
All resources, resource templates, and tools are registered at startup.

## Resources and templates

| URI | MIME type | Content |
| --- | --- | --- |
| `list://composition` | `application/json` | imported compositions |
| `tree://node/{composition}` | `application/json` | node tree of the composition |
| `tree://chunk/{composition}/{node}` | `application/json` | chunk tree organized by module, file chunks under `File` |
| `tree://item/{composition}/{node}` | `application/json` | item tree organized by domain |
| `tree://file/{composition}/{node}` | `application/json` | file tree organized by directory; a mapped directory is a branch that lists the host files below it, each readable as a `file://` resource |
| `tree://specialization/{composition}/{node}` | `application/json` | specialization tree of the node |
| `blocks://node/{composition}/{node}` | `application/json` | the node's own blocks (`Own`) |
| `blocks://apply/{composition}/{node}/{chunk}/{specialization}` | `application/json` | the blocks `-l` prints for that chunk reference |
| `blocks://effective/{composition}/{node}/{chunk}/{specialization}` | `application/json` | the blocks `-e` prints |
| `blocks://depend/{composition}/{node}/{chunk}/{specialization}` | `application/json` | the blocks `-d` prints |
| `blocks://refer/{composition}/{node}/{item}` | `application/json` | the blocks `-r` prints for that item |
| `blocks://specialization/{composition}/{node}/{labels}` | `application/json` | the blocks of the chain written under exactly that label path |
| `file://{composition}/{node}/{path}` | `text/plain` or `application/octet-stream` | evaluated bytes of that file |

- `list://composition` returns a JSON array of `{"name", "description", "ref"}` objects,
  where `name` is the composition directory's absolute path.
- `tree://` resources return recursive `{"name", "description", "ref", "sub-trees"}` JSON
  objects. Descriptions come from `composer.json` (for nodes) and from the reserved metadata
  chunks (for modules, chunks, specializations, files, directories, domains, and items).
- `tree://chunk` lists the chunk of a file block under `File`, one branch for each directory
  of its path (`File` → `cmd` → `main.go` for `cmd/main.go`). A `{chunk}` ref of such a leaf
  names the file's root-module chunk, so `blocks://` reads and `delete_chunk` act on the file's blocks.
- `blocks://` resources return a JSON array of `{"ref", "node", "comment", "text"}` objects.
  `text` is the block as written, without the blank after its end control; `comment` is the
  text before it, trimmed, and is left out when empty. `node` is the ref of the node that owns
  the block, and is left out when that is the node of the URI.
  A chunk reference with no matching block gives an empty array.
- Multiple compositions can be imported into one server. A ground composition appears in
  `list://composition` only if imported directly; its nodes inside a dependent composition
  can be inspected, evaluated, and used as bases, but attempting to edit them fails with
  `node '…' is defined in the ground`.

## Refs and tree structure

All `{composition}`, `{node}`, `{chunk}`, `{item}`, `{path}`, `{specialization}`, and `{labels}`
parameters in URIs, and the matching tool arguments, are refs.
A ref is the first ten hex digits of the SHA-256 of the object's kind and names,
separated by zero bytes, so it depends on nothing but what the object is called:

| Object | Kind | Names |
| --- | --- | --- |
| composition | `composition` | absolute path of its directory |
| node | `node` | names from the root to the node |
| module, chunk | `module`, `chunk` | path in the chunk tree |
| domain, item | `domain`, `item` | path in the item tree |
| directory, file | `directory`, `file` | path in the file tree |
| label path | `labels` | labels from general to specific |
| block | `block` | its text, without the blank after its end control |

- **Stability:** A ref stays the same across reads, mutations, and restarts, and changes only
  when the object is renamed; a block's ref changes when its text does. Two identical blocks
  of one node share a ref, and a tool takes the first.
- **Leaves vs. branches:** A branch and a leaf of the same path have different refs.
  `blocks://` and `file://` reads and the delete tools take leaves (chunks, items, files);
  `rename` takes either. In `tree://node` and `tree://specialization`, every entry is addressable.
- **Root wrappers:** The root of `tree://node` is the root node itself. The other trees
  (`chunk`, `item`, `file`, `specialization`) use a wrapper root object named `"/"`.
- **Specialization tree:** The root `"/"` represents no label. Each level below adds one label
  from general to specific, so the tree path `label1/label2` represents `label2@:label1`.
  A label path appears whenever a block header or a reference of the node's chain carries it.
  Any label path is accepted with any chunk; the blocks that match are those whose labels
  end the path.
- **Unknown refs:** A ref that names nothing fails with `unknown composition '…'`
  (or `node`, `chunk`, `file`, `item`, `specialization`, `block`, `object`).

## Tools

| Tool | Arguments | Effect |
| --- | --- | --- |
| `import_composition` | `path` | Load an existing `composer.json` (or its directory), again if already imported, and return its composition ref. |
| `create_composition` | `path`, `ground` (required: name or `null`), `description` (optional) | Create `path/composer.json` if absent, import it, and return its composition ref. Removes the created file if `ground` cannot be loaded. |
| `create_node` | `composition`, `name`, `archive`, `description`, `base` (required: parent node ref or `null` for root), `mappings` (optional: `path`, `origin`, `fileset` each) | Create a new empty archive file (fails with `archive '…' already exists` if present), add the node with its mappings to `composer.json`, and return the new node ref. A mapping that fails removes the node again. |
| `map_file` | `composition`, `node`, `path`, `origin`, `fileset` (optional) | Add a mapping to the node: the file or directory `origin`, relative to the composition directory, appears at `path` (a directory when `path` ends with `/`). Fails with `mapping '…' already exists`. |
| `delete_node` | `composition`, `node` | Delete the node, all descendant nodes based on it, and their archive files. Missing archive files are ignored; unremovable files are reported after saving `composer.json`. |
| `delete_chunk` | `composition`, `node`, `chunk` | Remove all of the node's own blocks for that module and chunk (across all label specializations) and compact the archive. |
| `delete_file` | `composition`, `node`, `file` | Remove the node's own file block and root-module blocks for that path and compact the archive. |
| `rename` | `composition`, `node`, `object` (ref of an entry of the chunk tree or the item tree), `to` (new path, such as `Module@/chunk`) | Move a chunk, every chunk of a module, an item, or every item of a domain to another path; the kind of the ref says which. Block headers, references, filters, items, and the descriptions under `Chunk@/` or `Index@/` follow, in the node and in the local nodes based on it; all labels are kept. A relative reference stays as written while it still leads to the same chunk, and is written from the root otherwise. |
| `operate_files` | `composition`, `node`, `command` | Interpret one shell command on the node's files and return a line per file changed. `rm [-r] [-f] PATH...` removes the own blocks of each file, the blocks of its root-module chunk, and its `File@/` description; a directory needs `-r`; a path that one of the node's mappings holds removes that mapping. `mv [-f] SOURCE... TARGET` moves files or directories: file headers, references to the file, file operands of views (`@,` and `@!`), and `File@/` descriptions follow, in the node and in the local nodes based on it. |
| `evaluate_expression` | `composition`, `node`, `expression` | Evaluate a `-p` expression at that node and return the resulting text. |
| `apply_block` | `composition`, `node`, `text`, `overrides` (optional block ref) | Append `text` to the node's archive, reparse, and compact the archive. With `overrides`, the blocks of `text` take the place of that block instead; each must have its header reference, or the call fails with `block '…' is not applied to '…'`. The comment of the overridden block goes with it, and the new text starts after the same blank lines. Returns the refs of the blocks of `text`, one per line. |
| `delete_block` | `composition`, `node`, `block` | Remove one of the node's own blocks and compact the archive. |
| `move_block` | `composition`, `node`, `block`, `to` (node ref) | Take one own block, with its comment, to the end of another local node, and compact both archives; neither is written unless both can be. |
| `describe` | `composition`, `node` (optional), `description` | Replace the description of the node, or of the composition when no node is given, and rewrite the composer file. Returns the ref of what was described. |
| `search_blocks` | `composition`, `node`, `text` | Return, as a `blocks://` array, the blocks of the node's chain whose comment or text contains `text`. More than 100 matches is an error. |

`rename` and `operate_files` change names in place, so each block keeps its content and comment.
They refuse a change that would lose or split something, and then leave every archive untouched:

- **`rename`:** `unknown chunk '…'` (or `module`, `item`, `domain`) when the node itself has no such block;
  `chunk '…' is defined in node '…'` when a base node holds it, since a base keeps its blocks,
  and likewise `item '…' is used in node '…'`;
  `chunk '…' already exists` when another chunk holds a new path (items may merge);
  `'…' is reserved` for the namespaces `Chunk`, `Specialization`, `File`, and `Index`;
  `'…' is a file` for a file chunk, which `operate_files` moves.
- **`operate_files`:** `'…' comes from node '…'` or `'…' comes from a mapping` when the node does not
  hold the file itself; `file '…' already exists` when `mv` would land on another file
  (unlike a shell, it never replaces one); `'…' is a directory` for `rm` without `-r`;
  `no such file '…'` (ignored by `rm -f`); `target '…' is not a directory`;
  `cannot move '…' into itself`; `invalid path '…'` for a path that is absolute or leaves the root.
  Words are split as a shell splits them, with quotes and backslashes; patterns are not expanded.

## Compaction and notifications

Every archive edit (`apply_block`, `delete_block`, `move_block`, `delete_chunk`, `delete_file`, `rename`,
`operate_files`) compacts the edited node's archive before writing it back:

1. **Drop overridden blocks:** Any block in `Own` that is no longer an effective block of
   its own chunk reference within `Own` (because a later `@=` in the same node replaced it)
   is removed.
2. **Order by inclusion:** Remaining blocks are ordered so that a block appears after any
   block whose body content references its chunk (matched by module and chunk name regardless
   of labels; filter references and self-references are ignored). Blocks targeting the same
   module and chunk always keep their relative order; if inclusion forms a cycle, archive
   order breaks the tie; otherwise blocks keep their relative order. Each block's preceding
   `Comment` moves with it, and a block that previously ended the archive at a bare `@`
   receives a trailing `\n` if moved before another block.

Read operations never compact archives. (`DumpArchives` runs the same compaction across all
local nodes in a `World`, though it is not exposed as an MCP tool.)
Saving `composer.json` writes only `description`, `ground`, and the local nodes declared in
that composition.
Every mutation sends `notifications/resources/updated` for `list://composition` and for
every concrete resource URI that existed before or after the change.

# Tests

- **Unit tests (`internal/litar/*_test.go` in the node `Litar`, in the file set `go tests`, not synced into `src/`):** Table-driven tests covering scanning and
  parsing, suffix matching, label inheritance, `.` and `..` module resolution, effective
  blocks, listings, metadata chunks, filters, FUSE views, composer edits, compaction
  ordering, and MCP calls.
- **Acceptance tests (`Project@/litar@/Tests` in the node `Litar Build`):** each case is a chunk under
  `Project@/litar@/Tests@/Cases` whose content is a bash script of checks, filtered by
  `litar command@:UI@/Command Line@/Test@/run case` in a view of the file sets `binaries`
  and `test fixtures`; the value of the chunk is the result of the case
  (`ok NAME COUNT`, `skip NAME: REASON`, or `FAIL NAME` followed by the failed checks),
  so a failed case still evaluates. `Project@/litar@/Tests@/case results` lists the cases,
  and `Project@/litar@/Tests@/acceptance report` pipes them through
  `UI@/Command Line@/Test@/summarize results`, which fails when a case failed or none passed.
  The fixtures are the files under `tests/cases/` of the node `Litar Build`; they are not synced into `src/`.
  `Project@/litar@/Tests@/go test report` is the report of the unit tests.
- **The engine (`UI@/Command Line@/Test` in `standard`)** is shared by every command.
  A project sets `command` (the path of the command in the view) and `environment`
  (shell lines) under a label, and passes that label to `run case`.
  A case script calls `test_case NAME [DIRECTORY]`, then the checks
  `prints CHECK TEXT ARGUMENT...`, `prints_file CHECK FILE ARGUMENT...`, and
  `fails CHECK NEEDLE ARGUMENT...`, or `skip REASON`; `run ARGUMENT...` runs the command
  for a check of the project's own, added to the chunk `checks`.

# Grounds and examples

- `standard`: the standard library, one node `Standard` whose modules follow
  `design/category.md`: `Tool` (shared filters, including `Tool@/Text Processing`),
  `Language@/C@/Module`, `Language@/Go@/File`, `Language@/Go@/Module`,
  `UI@/Command Line` (the invocation syntax, its generators for C and Go, and the acceptance test engine `Test`),
  and `Framework@/Light`.
- `litar`: grounds on `standard` and adds `Litar`, whose chunks live under `Project@/litar`,
  and `Litar Build`, based on `Litar`, whose chunks live under `Project@/litar@/Build`.
- `examples/c`: grounds on `standard` and adds `Program Hello` and `Test Light`.
- `examples/go`: grounds on `standard` and adds `Program Hello` and `Project Hello`.

The ground `standard` sits at the repository root, so litar finds it when run from there;
elsewhere, put the repository root in `LITAR_LIBRARY`.
