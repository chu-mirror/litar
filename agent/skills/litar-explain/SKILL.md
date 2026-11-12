---
name: litar-explain
description: Explain an item, chunk, or file of a litar composition by locating the blocks that define and use it through the litar MCP server, then describing what it does and how it connects to its neighbours. Use when the user asks to explain, describe, or walk through an item (such as `Project@/litar@/Go@/functions@/litar.run`), a chunk, or a generated file in a litar composition.
---

# Explaining litar items

Read-only: never apply, delete, rename, or move while explaining.
Resources and refs: [mcp.md](../references/mcp.md). Syntax: [language.md](../references/language.md).
Item paths: Items in [style.md](../references/style.md).

## Workflow

1. **Load.** Read `list://composition`; if the composition is missing,
   `import_composition` its directory and keep the ref it returns.
2. **Pick the node.** `tree://node/{c}`; use the leaf node unless the user names one.
3. **Find the item.** `tree://item/{c}/{n}` and walk `owner → kind → name`
   (e.g. `Project` → `litar` → `Go` → `functions` → `litar.run`).
   Its description comes from an `Index@/…` block.
4. **Get its blocks.** `blocks://refer/{c}/{n}/{item}`. The block whose code defines the item
   is the definition; the others are its users. A block with a `node` field lives in a base node.
5. **Follow one level out.** For items the definition itself carries (callees, types, variables),
   read their `blocks://refer` too, but only as far as the explanation needs.
6. **Explain.** Answer, then support it.

For a chunk instead of an item: `tree://chunk`, then `blocks://effective` (what produces it)
and `blocks://depend` (who uses it), with the specialization ref of its labels.
For everything written under one label path, such as one source file: `blocks://specialization`.
For a file: `file://{c}/{n}/{file}`.

## Answer shape

- Open with one sentence: what the item is and does.
- Say where it lives: node, then chunk as `label@:Module@/chunk`, and how many blocks carry it.
- Show the defining code, then explain its steps in prose.
- Use the comments of the blocks: they record the decisions behind the code.
- Explain each user block: who calls or reads it and why.
- Use litar terms (node, chunk, block, item) and copy names exactly.
