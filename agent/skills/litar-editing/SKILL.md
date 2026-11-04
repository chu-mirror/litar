---
name: litar-editing
description: Explore and edit litar compositions through the litar MCP server — find nodes, chunks, files and blocks, apply or delete blocks, evaluate expressions, and verify generated files. Use when reading or modifying litar archives (.la), composer.json compositions, grounds, or examples, or when the user mentions chunks, blocks, nodes, or apply_block.
---

# Editing litar compositions

Never read or edit `.la` archives directly; go through the `litar` MCP server.

References (read when needed):
- [language.md](../references/language.md): archive syntax, newline rule, specialization, files, items.
- [mcp.md](../references/mcp.md): resources, tools, ids, compaction.
- [style.md](../references/style.md): how to name chunks and shape blocks.

## Explore

1. Read `list://composition`; `import_composition` the directory if it is missing.
   Wait for the import to return before reading its resources.
2. `tree://node/{c}` for the node chain, then `tree://chunk`, `tree://file`, `tree://item`.
3. `blocks://node/{c}/{n}` for a node's own blocks with their comments.
4. For one chunk: `tree://specialization`, then `blocks://effective` (what produces it)
   and `blocks://depend` (who uses it).
5. `evaluate_expression` or `file://` for the result.
   Save large results to a temporary file to search them; remove it afterwards.

To trace a line of generated output back to its block, evaluate the file, find the line,
then look through `blocks://node` of each node along the base chain;
it is often in a base node, not the leaf.

## Edit

- Copy the exact header from `blocks://`, labels included.
- Replace a chunk: `apply_block` a `@=` block; older own blocks of that reference are dropped.
- Extend a chunk: `apply_block` a `@+` block.
- **Change or remove one `@+` block.** Re-applying it would add a duplicate. Instead:
  - re-apply an `@=` block holding the content of the blocks you keep before it,
    followed by the remaining `@+` blocks; or
  - read every own block of that module and chunk (all labels, in order, with comments),
    `delete_chunk`, then re-apply them all with the change.
- Keep comments and bodies byte-for-byte except for the intended change.
- Do not re-apply blocks to move them; compaction decides the order.
- Ground nodes are read-only in a composition that grounds on them;
  import the ground composition itself to edit them.
- New work goes in a task node; see the [litar-task](../litar-task/SKILL.md) skill.

## Verify

1. Re-evaluate the changed chunk or file and compare with the intent.
2. Write generated programs to a temporary directory, then build and run them,
   or evaluate the composition's own check chunk.
3. `git diff` the archive: only the intended blocks changed (plus reordering).

## Pitfalls

- After `./litar` is rebuilt, the running MCP server is stale; ask the user to restart it
  before relying on its evaluation or compaction.
- If a change cannot be expressed through the tools, ask the user before editing the file.
