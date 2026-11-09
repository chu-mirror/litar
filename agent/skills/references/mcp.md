# litar MCP server

Started as `litar -a` (optionally `-f ARCHIVE` to import that archive's composition),
with `LITAR_LIBRARY` listing the ground libraries.

## Ids

- Composition id: index in import order (`list://composition`).
- Node, chunk, item, file, specialization ids: preorder walk of their tree, from 0.
  They change after mutations; fetch the tree again.
- Chunk, item, and file ids address leaves only.
- Specialization id 0 is the unlabelled chunk; a child path `label1/label2` is `label2@:label1`.

## Resources

| URI | Returns |
| --- | --- |
| `list://composition` | imported compositions |
| `tree://node/{c}` | node tree |
| `tree://chunk/{c}/{n}` | chunks by module, file chunks under `File`, with descriptions |
| `tree://item/{c}/{n}` | items by domain, with descriptions |
| `tree://file/{c}/{n}` | files by directory |
| `tree://specialization/{c}/{n}/{chunk}` | label paths used for that chunk |
| `blocks://node/{c}/{n}` | the node's own blocks, with comments |
| `blocks://apply/{c}/{n}/{chunk}/{spec}` | all blocks matching that reference |
| `blocks://effective/{c}/{n}/{chunk}/{spec}` | blocks still contributing |
| `blocks://depend/{c}/{n}/{chunk}/{spec}` | blocks that include or filter by it |
| `blocks://refer/{c}/{n}/{item}` | blocks carrying that item |
| `file://{c}/{n}/{path}` | evaluated file bytes |

## Tools

| Tool | Arguments | Effect |
| --- | --- | --- |
| `import_composition` | `path` | load a composition; returns its id |
| `create_composition` | `path`, `ground`, `description` | create `composer.json` and import it |
| `create_node` | `composition`, `name`, `archive`, `description`, `base` | new node with a new empty archive; `base` null = root |
| `delete_node` | `composition`, `node` | delete it, its descendants, and their archives |
| `delete_chunk` | `composition`, `node`, `chunk` | delete own blocks of that module+chunk, all labels |
| `delete_file` | `composition`, `node`, `path` | delete own blocks of that file |
| `evaluate_expression` | `composition`, `node`, `expression` | evaluate a qualified name with an optional pipeline, e.g. `hello.c` |
| `apply_block` | `composition`, `node`, `text` | append the block text to the node's archive |

## Compaction

Every edit rewrites the node's archive:

1. blocks that are no longer effective for their own reference are dropped
   (so an `@=` replaces the older own blocks of that reference);
2. a block is moved after the blocks whose content includes its chunk;
   blocks of the same module+chunk keep their order; comments move with their block.

Reads never compact.

## Errors worth knowing

- `node '…' is defined in the ground`: the node came through `ground`; import the ground
  composition itself to edit it.
- `unknown composition N` / `unknown node N` / …: stale id; re-read the list or tree.
