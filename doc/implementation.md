# Implementation

This document describes the Go implementation of `litar`: its package layout, data model,
parser, composer loader, evaluator, filter execution, command-line interface, and MCP server.
See [syntax.md](syntax.md) for the archive language and `composer.json` specification.

# Layout

The Go module `litar` (Go 1.25, formatted with `gofmt`) lives under `src/` and is generated
from the litar composition in `compos/`:

- `cmd/litar/main.go`: entry point that passes `os.Args` to `litar.Main`.
- `cmd/litar-web/main.go`: read-only web UI that spawns `litar -a` and serves its MCP
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

External dependencies are limited to `github.com/hanwen/go-fuse/v2` (for filter view mounts)
and `github.com/modelcontextprotocol/go-sdk` (for the MCP server).

At the repository root, `make` evaluates `compos/litar.la` into `tmp/build` and compiles
`./litar` and `./litar-web`, while `make sync` writes all generated files into `src/`.
Inside `src/`, `make` builds `./litar` and `make test` runs `go test ./...` followed by
`tests/run.sh`.
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
`list.go`). They omit the reserved namespaces (`Module`, `Chunk`, `Specialization`, `Domain`,
`Index`) and populate descriptions by evaluating those metadata chunks.

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
usage: litar [options]

--help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-f, --file ARCHIVE: specify the archive file, find the composition and the node corresponding
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
```

- `--help` prints the usage above (with `argv[0]` in place of `litar`) and is long-only.
- Short options, long options, attached short values (`-fARCHIVE`), and `--name=value` are
  all accepted.
- `ARCHIVE` is a path to a `.la` archive file; `CHUNK_REFERENCE`, `EXPRESSION`, and `ITEM`
  follow the command-line syntax in [syntax.md](syntax.md).
- Exactly one action (`-p`, `-l`, `-e`, `-d`, `-r`, or `-a`) may be given per invocation.
  Combining two actions fails with `cannot combine <first> and <second>`, naming the earlier
  action in the order `print`, `literal`, `effective`, `depend`, `reference`, `agent`.
- `-f` is required for every action except `--help` and `-a`.
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
| `tree://chunk/{composition}/{node}` | `application/json` | chunk tree organized by module |
| `tree://item/{composition}/{node}` | `application/json` | item tree organized by domain |
| `tree://file/{composition}/{node}` | `application/json` | file tree organized by directory |
| `tree://specialization/{composition}/{node}/{chunk}` | `application/json` | specialization tree of a chunk |
| `blocks://node/{composition}/{node}` | `text/plain` | the node's own blocks (`Own`) |
| `blocks://apply/{composition}/{node}/{chunk}/{specialization}` | `text/plain` | `-l` output for that chunk reference |
| `blocks://effective/{composition}/{node}/{chunk}/{specialization}` | `text/plain` | `-e` output for that chunk reference |
| `blocks://depend/{composition}/{node}/{chunk}/{specialization}` | `text/plain` | `-d` output for that chunk reference |
| `blocks://refer/{composition}/{node}/{item}` | `text/plain` | `-r` output for that item |
| `file://{composition}/{node}/{path}` | `text/plain` or `application/octet-stream` | evaluated bytes of that file |

- `list://composition` returns a JSON array of `{"name", "description", "id"}` objects,
  where `name` is the composition directory's absolute path.
- `tree://` resources return recursive `{"name", "description", "id", "sub-trees"}` JSON
  objects. Descriptions come from `composer.json` (for nodes) and from the reserved metadata
  chunks (for modules, chunks, specializations, domains, and items).
- `blocks://` resources return blocks formatted identically to `-l`.
- Multiple compositions can be imported into one server. A ground composition appears in
  `list://composition` only if imported directly; its nodes inside a dependent composition
  can be inspected, evaluated, and used as bases, but attempting to edit them fails with
  `node '…' is defined in the ground`.

## IDs and tree structure

All `{composition}`, `{node}`, `{chunk}`, `{item}`, `{path}`, and `{specialization}`
parameters in URIs and tool calls are integer IDs counting from `0`:

- **Composition ID:** Index in import order (`list://composition`).
- **Tree IDs:** Assigned by a preorder walk of the corresponding tree starting at `0`.
- **Leaves vs. branches:** In `tree://chunk`, `tree://item`, and `tree://file`, only leaf
  entries (actual chunks, items, and files—not intermediate module, domain, or directory
  nodes) are valid IDs for tools and `blocks://` / `file://` reads. In `tree://node` and
  `tree://specialization`, every node in the tree is addressable.
- **Root wrappers:** The root of `tree://node` is the root node itself. The other trees
  (`chunk`, `item`, `file`, `specialization`) use a wrapper root object named `"/"`.
- **Specialization tree:** In `tree://specialization/{composition}/{node}/{chunk}`, the
  root `"/"` (ID `0`) represents the unspecialized chunk. Each level below adds one label
  from general to specific, so the tree path `label1/label2` represents `label2@:label1`.
  A label path appears in the tree whenever a block is applied to, or a reference resolves
  to, that chunk with those labels.
- **Unknown IDs:** Passing an out-of-range or non-leaf ID fails with `unknown composition N`
  (or `node`, `chunk`, `file`, `item`, `specialization`).

## Tools

| Tool | Arguments | Effect |
| --- | --- | --- |
| `import_composition` | `path` | Load an existing `composer.json` (or its directory) and return its composition ID. |
| `create_composition` | `path`, `ground` (required: name or `null`), `description` (optional) | Create `path/composer.json` if absent, import it, and return its composition ID. Removes the created file if `ground` cannot be loaded. |
| `create_node` | `composition`, `name`, `archive`, `description`, `base` (required: parent node ID or `null` for root) | Create a new empty archive file (fails with `archive '…' already exists` if present), add the node to `composer.json`, and return the new node ID. |
| `delete_node` | `composition`, `node` | Delete the node, all descendant nodes based on it, and their archive files. Missing archive files are ignored; unremovable files are reported after saving `composer.json`. |
| `delete_chunk` | `composition`, `node`, `chunk` | Remove all of the node's own blocks for that module and chunk (across all label specializations) and compact the archive. |
| `delete_file` | `composition`, `node`, `path` | Remove the node's own file block and root-module blocks for that path and compact the archive. |
| `evaluate_expression` | `composition`, `node`, `expression` | Evaluate a `-p` expression at that node and return the resulting text. |
| `apply_block` | `composition`, `node`, `text` | Append `text` to the node's archive, reparse, and compact the archive. |

## Compaction and notifications

Every archive edit (`apply_block`, `delete_chunk`, `delete_file`) compacts the edited node's
archive before writing it back:

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

- **Unit tests (`src/internal/litar/*_test.go`):** Table-driven tests covering scanning and
  parsing, suffix matching, label inheritance, `.` and `..` module resolution, effective
  blocks, listings, metadata chunks, filters, FUSE views, composer edits, compaction
  ordering, and MCP calls.
- **Acceptance tests (`src/tests/`):** CLI test cases driven by `run.sh`. Each case
  directory contains either a custom `driver.sh` or an `argv` file (one CLI argument per
  line, including `-f`) paired with expected `out`, `err`, and `status` files.

# Grounds and examples

- `grounds/common`: defines `Basic` (including `Text Processing`).
- `grounds/c-programming`: grounds on `common` and adds `C Structure` and `Light`.
- `grounds/go-programming`: grounds on `common` and adds `Go Structure` and `Go Project`
  (based on `Go Structure`).
- `examples/c`: grounds on `c-programming` and adds `Program Hello` and `Test Light`.
- `examples/go`: grounds on `go-programming` and adds `Program Hello` (on `Go Structure`)
  and `Project Hello` (on `Go Project`).

Set `LITAR_LIBRARY=grounds` when evaluating compositions that depend on `grounds/`.
