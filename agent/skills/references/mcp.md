# litar MCP server

Started as `litar -a` (optionally `-f ARCHIVE` to import that archive's composition),
with `LITAR_LIBRARY` listing the ground libraries.
Never read or edit `.la` archives directly; go through this server.

## Refs

Every object is addressed by its `ref`, ten hex digits hashed from its kind and its names
(a block: from its text). Resource URIs and tool arguments take refs.

- A ref stays the same across reads, mutations, and restarts; it changes only when
  the object is renamed (a block: when its text changes). Keep refs; do not fetch a tree again
  just to use one.
- A branch and a leaf of the same path (a module and a chunk, a domain and an item)
  have different refs.
- A specialization ref names a label path of the node; the root of the tree is "no label".
  Any label path is accepted for any chunk: it shows the blocks of the paths it ends with.
- `apply_block` returns the refs of the blocks it applied.

## Resources

| URI | Returns |
| --- | --- |
| `list://composition` | imported compositions |
| `tree://node/{c}` | node tree |
| `tree://chunk/{c}/{n}` | chunks by module, file chunks under `File`, with descriptions |
| `tree://item/{c}/{n}` | items by domain, with descriptions |
| `tree://file/{c}/{n}` | files by directory |
| `tree://specialization/{c}/{n}` | label paths used in the node; `a` → `b` is `b@:a` |
| `blocks://node/{c}/{n}` | the node's own blocks |
| `blocks://apply/{c}/{n}/{chunk}/{spec}` | all blocks matching that reference |
| `blocks://effective/{c}/{n}/{chunk}/{spec}` | blocks still contributing |
| `blocks://depend/{c}/{n}/{chunk}/{spec}` | blocks that include or filter by it |
| `blocks://refer/{c}/{n}/{item}` | blocks carrying that item |
| `blocks://specialization/{c}/{n}/{spec}` | blocks written under exactly that label path |
| `file://{c}/{n}/{file}` | evaluated file bytes |

A list entry is `{name, description, ref}`; a tree entry adds `sub-trees`.
A `blocks://` resource is a JSON array of `{ref, node, comment, text}`:
`node` is the ref of the owning node, left out for the node of the URI;
`comment` is left out when empty.

Read narrowly. `blocks://node` of a large node is thousands of lines:
prefer `blocks://effective`, `blocks://refer`, or `blocks://specialization`
(one label path is often one source file).

## Tools

| Tool | Arguments | Effect |
| --- | --- | --- |
| `import_composition` | `path` | load a composition, or load it again from disk; returns its ref |
| `create_composition` | `path`, `ground`, `description` | create `composer.json` and import it |
| `create_node` | `composition`, `name`, `archive`, `description`, `base`, `mappings` | new node with a new empty archive; `base` null = root |
| `map_file` | `composition`, `node`, `path`, `origin`, `fileset` | map a file or directory (`path` ending in `/`) outside the archives into the node |
| `delete_node` | `composition`, `node` | delete it, its descendants, and their archives |
| `delete_chunk` | `composition`, `node`, `chunk` | delete own blocks of that module+chunk, all labels |
| `delete_file` | `composition`, `node`, `file` | delete own blocks of that file |
| `rename` | `composition`, `node`, `object`, `to` | rename a chunk, module, item, or domain (`object` is its ref, `to` the new path); headers, references, filters, items, and descriptions follow in the node and the nodes based on it |
| `operate_files` | `composition`, `node`, `command` | `rm [-r] [-f] PATH...` or `mv SOURCE... TARGET` on the node's files; references, view operands, and descriptions follow; `rm` of a mapped path removes the mapping |
| `evaluate_expression` | `composition`, `node`, `expression` | evaluate a qualified name with an optional pipeline, e.g. `hello.c` |
| `apply_block` | `composition`, `node`, `text`, `overrides` | append the block text (comment first); with `overrides` (a block ref) the new text takes that block's place |
| `delete_block` | `composition`, `node`, `block` | delete one block |
| `move_block` | `composition`, `node`, `block`, `to` | move one block, with its comment, to another node of the composition |

## Editing

- **Change a block:** `apply_block` with `overrides`. The new block must have the same header
  reference; it keeps the position, so the order of `@+` blocks is preserved.
  The old comment goes; put the comment you want before the new block.
- **Set a chunk:** `apply_block` a `@=` block; the older own blocks of that reference are dropped.
- **Extend a chunk:** `apply_block` a `@+` block.
- **Rename or move** with `rename` and `operate_files`, never by re-applying under a new name.
- Copy headers exactly from `blocks://`, labels included.
- Ground nodes are read-only in a composition grounded on them; import the ground itself.
- After an edit, evaluate the changed chunk or file, then build or run the result.

## Compaction

Every edit rewrites the node's archive:

1. blocks that are no longer effective for their own reference are dropped
   (so an `@=` replaces the older own blocks of that reference);
2. a block is moved after the blocks whose content includes its chunk;
   blocks of the same module+chunk keep their order; comments move with their block.

Reads never compact.

## Errors worth knowing

- `node '…' is defined in the ground`: import the ground composition itself to edit it.
- `unknown composition '…'` / `unknown node '…'` / `unknown block '…'`: the object was renamed,
  changed, or removed; read the list, tree, or blocks again.
- `block '…' is not applied to '…'`: an override must keep the header reference;
  use `delete_block` and `apply_block` to move content to another chunk.
- `chunk '…' is defined in node '…'`: rename it in that base node; the nodes based on it follow.
- `'…' comes from node '…'` / `'…' comes from a mapping`: the node does not hold that file itself.
- `file '…' already exists`: `mv` never replaces a file; `rm` the target first.
- A failed evaluation reports only the stderr of its filters; their stdout is dropped.
- A change made to an archive outside the server (a script, version control) is not seen
  until `import_composition` is called again.
