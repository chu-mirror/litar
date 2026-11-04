# Implementation

This describes the Go program `litar`:
its structure, the evaluation rules, the command line, and the MCP server.
[syntax.md](syntax.md) defines the archive language and the composer file.

# Layout

Module `litar`, Go 1.25, `gofmt` on every file.
`cmd/litar/main.go` only calls `litar.Main`;
everything else is the single package `internal/litar`, split by concern:
`types.go`, `parse.go`, `compose.go`, `eval.go`, `list.go`,
`filter.go`, `fuse.go`, `cli.go`, `mcp.go`.

Dependencies: the standard library,
`github.com/hanwen/go-fuse/v2` for views,
and `github.com/modelcontextprotocol/go-sdk` for the server.

`make` builds `./litar`; `make test` runs `go test ./...` and then `tests/run.sh`.
A user error goes to stderr, exits 1, and leaves stdout empty.

# Data

See `types.go`. The points that the rest of the program relies on:

- a `Block` keeps `Comment` (the bytes after the previous expression) and `Raw`
  (the expression as written, with its end control),
  so `-l` and archive rewrites reproduce the source byte for byte;
  `Trailing` on a node or archive holds the bytes after the last block;
- `EndsLine` records that a line break ended the content;
- a file is a `Block` with `IsFile`; its chunk is its path in the root module;
- a node's `Own` is its archive's blocks; its chain is the base node's chain followed by `Own`.
  Evaluation and listings use the chain; edits, dump, and `blocks://node` use `Own`.

# Parse

Parse with a byte cursor over the controls of [syntax.md](syntax.md), not regular expressions.

- The first CR, LF, or CR LF after `@=` / `@+` is layout.
- One LF or CR LF that ends the content, before `@`, `@|`, or `@~`, is layout
  and sets `EndsLine`.
- `@<` raises the body depth and `@>` lowers it; `@|` and `@~` start the pipeline
  and the items only at depth 0. An end control ends the block at any depth.
- A command-line name uses the same qualified-name parser on the whole argument;
  `-p` allows a pipeline and views, `-l`, `-e`, and `-d` do not.

Errors name the file, the line, and the unexpected control.
File errors are `files are paired only with @=`, `files cannot be specialized`,
and `files cannot be modularized`.

# Composer

The format and the ground search order are in [syntax.md](syntax.md).

- The ground composer loads first; its nodes, including its own ground's, precede this file's.
  The combined list has exactly one root.
- A base must name a node already in the list:
  `base '…' of node '…' is not defined`. A cycle among grounds names the grounds.
- Each archive is parsed once into `Own`. A missing archive fails the load.
- File-set membership is the `Sets` on file blocks in the chain plus the mappings
  inherited along it; `@+name` adds every file and mapping that lists `name`.
- `-n` selects the node; without it, the root.

# Evaluation

A block matches a chunk reference when module and chunk are equal
and the block's labels are a suffix of the reference's labels.
Matching blocks apply in chain order:

- `@=` replaces the accumulated text;
- `@+` appends, joined by a line feed when the block has `EndsLine`
  and the accumulated text is not empty.

The blocks that still contribute are the effective blocks.
Each applied block expands its parts, runs its pipeline on that text, then sets or appends.
Filter output is final and is not scanned again.

- An inner reference without labels takes the labels of the reference being expanded.
- An omitted module is the root. `.` and `..` resolve against the block's header module
  and are an error in a header or a filter name.
- A filter name resolves like a body reference, without `.` or `..`.
- A missing inner reference is empty text; a missing top-level reference is an error.
- Re-expanding a reference already on the visit stack is a cycle; stderr names the chain.

# Listing

`-l` prints the comment and `Raw` of each matching block, predecessors included;
`-e` does the same for the effective blocks.
`-d` lists blocks whose content contains, or whose pipeline names,
the query or a reference specialized from it.
`-r` lists blocks that carry exactly that item.

Module, chunk, domain, and item listings exist only behind the MCP trees.
They skip the reserved namespaces and take descriptions from the meta chunks.

# Filters

A filter chunk evaluates to a script whose first line is a shebang.
The script is written to a host temporary file;
stdin is the text, stdout replaces it, stderr passes through,
and a non-zero status fails the evaluation.

Before the process starts, the view is computed from the empty set
left to right, and every file in it is tangled:
an archive file is its evaluated chunk, on the same visit stack;
a mapped file is the contents of `origin`.
A cycle or an unreadable file fails the invocation before anything runs.

The view is a FUSE mount containing exactly those files, read-only,
with stats fixed at tangle time.
Writes go to a host overlay visible through the mount and discarded afterwards.
The process runs with the mount as its working directory; absolute host paths stay reachable.

Constraints the mount imposes:

- the child enters the mount through `/bin/sh -c 'cd "$0" && exec "$@"'`,
  never through `cmd.Dir`: Go's vfork child would `chdir` into this process's own
  FUSE mount and deadlock with a garbage-collector stop;
- if unmounting fails because a background process still uses the mount,
  it is detached lazily instead of waited for;
- SIGINT, SIGTERM, and SIGHUP detach every live mount before exiting.

Tests that need a mount skip when `/dev/fuse` is missing.

# Command line

```
usage: litar [options]

--help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-t, --target COMPOSITION: specify the composition as COMPOSITION
-n, --node NODE...: specify the node pointed to by path NODE...
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
```

The help text is printed as above, with `argv[0]` for `litar`.
COMPOSITION is a directory containing `composer.json`;
NODE is the node names from the root;
CHUNK_REFERENCE, EXPRESSION, and ITEM are the command-line names of [syntax.md](syntax.md).
`--help` is long only; short options, long options, and `--name=value` are accepted.
One action per invocation: combining two of `-p -l -e -d -r -a` is an error
naming the earlier one in that order.
`-t` is required except for `--help` and `-a`, and must be a directory.
`-a` without `-t` starts the server with nothing imported.

# MCP

`-a` serves MCP protocol
[2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28) on stdio.
Every resource, template, and tool is registered at startup.

| URI | Content |
| --- | --- |
| `list://composition` | the imported compositions |
| `tree://node/{composition}` | the node tree |
| `tree://chunk/{composition}/{node}` | the chunk tree, by module |
| `tree://item/{composition}/{node}` | the item tree, by domain |
| `tree://file/{composition}/{node}` | the file tree, by directory |
| `tree://specialization/{composition}/{node}/{chunk}` | the specializations of a chunk |
| `blocks://node/{composition}/{node}` | the node's own blocks |
| `blocks://apply/{composition}/{node}/{chunk}/{specialization}` | `-l` of that chunk reference |
| `blocks://effective/{composition}/{node}/{chunk}/{specialization}` | `-e` of that chunk reference |
| `blocks://depend/{composition}/{node}/{chunk}/{specialization}` | `-d` of that chunk reference |
| `blocks://refer/{composition}/{node}/{item}` | `-r` of that item |
| `file://{composition}/{node}/{path}` | the evaluated bytes of that file |

`list://` returns a JSON array of `{"name", "description", "id"}`;
`tree://` returns `{"name", "description", "id", "sub-trees"}`;
`blocks://` returns the `-l` form.
A composition's name is its absolute path; descriptions come from the composer and the meta chunks.

Several compositions can be imported into one server.
A ground composition is listed only when imported itself;
its nodes inside another composition can be evaluated and used as bases but not edited
(`node '…' is defined in the ground`).

Ids:

- a composition id is its index in import order;
- node, chunk, item, file, and specialization ids are a preorder walk of their tree;
- chunk, item, and file ids address leaves, not module or directory branches;
- an unknown id is `unknown composition N` (or node, chunk, file, item, specialization).

Trees:

- the node tree's root object is the root node; the other trees use `"/"` as the wrapper;
- reserved meta chunks are omitted from `tree://chunk`;
  module and chunk descriptions appear there, domain and item descriptions on `tree://item`;
- `tree://specialization/{composition}/{node}/{chunk}` has the unspecialized form as root (id 0),
  then one label per level, general to specific, so `label1/label2` is `label2@:label1`.
  A label path appears when a block applies to the chunk, or a reference names it,
  under those labels; a reference without labels keeps its block's labels.

Tools and their arguments:

- `import_composition`: `path`, the composer file or its directory; returns the composition id;
- `create_composition`: `path`, `ground` (required, name or null), `description`;
  creates `composer.json` if absent, removes it again if the ground cannot load;
- `create_node`: `composition`, `name`, `archive`, `description`, `base`
  (required; null creates the root, only when there is none);
  the archive is created empty and must not exist (`archive '…' already exists`);
- `delete_node`: `composition`, `node`; also deletes nodes based on it and their archives;
  a missing file is not an error, a file that cannot be removed is reported after saving;
- `delete_chunk`: `composition`, `node`, `chunk`; every own block of that module and chunk,
  whatever the labels;
- `delete_file`: `composition`, `node`, `path`;
- `evaluate_expression`: `composition`, `node`, `expression`;
- `apply_block`: `composition`, `node`, `text`, appended to the node's archive.

After an edit the node's archive is compacted:

1. drop a block that is no longer an effective block of its own chunk reference in that node;
2. order the rest so a block comes after the blocks whose content includes its chunk.
   Inclusion is a content reference resolved like a body reference,
   matched by module and chunk whatever the labels; filter chunks and self-inclusion do not count.
   Blocks of one module and chunk keep their order; circular inclusion keeps archive order;
   other blocks keep their relative places; comments move with their blocks;
   a block that ended the archive at its `@` gets a line break when it moves.

Reads never compact. `DumpArchives` applies the same compaction to every local node; it is not a tool.
Saving the composer writes `description`, `ground`, and the local nodes only.
A mutation sends `notifications/resources/updated` for `list://composition`
and every concrete URI present before or after it.

# Tests

`internal/litar` has table tests for parsing, matching, label inheritance, `.` / `..`,
effective blocks, listings, filters, FUSE views, composer edits, and MCP.
`tests/` holds CLI cases run by `run.sh`; a case is a directory with `driver.sh` or `argv`
(one argument per line, including `-t`).

# Grounds and examples

`grounds/common` is Basic, which also holds Text Processing.
`grounds/c-programming` adds C Structure and Light;
`grounds/go-programming` adds Go Structure, then Go Project on it.
`examples/c` adds Program Hello and Test Light on `c-programming`;
`examples/go` adds Program Hello on Go Structure and Project Hello on Go Project.
Put `grounds` on `$LITAR_LIBRARY` to evaluate them.
