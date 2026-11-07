---
name: litar-explain
description: Explain an item, chunk, or file of a litar composition by locating the blocks that define and use it through the litar MCP server, then describing what it does and how it connects to its neighbours. Use when the user asks to explain, describe, or walk through an item (such as `Go@/functions@/pkg.Name`), a chunk, or a generated file in a litar composition.
---

# Explaining litar items

Read-only: never apply, delete, or compact while explaining.
Tools and ids: [mcp.md](../references/mcp.md). Item syntax: Items in [language.md](../references/language.md).

## Workflow

1. **Load.** Read `list://composition`; if the composition is missing,
   `import_composition` its directory (find its `composer.json` by name) and wait for the id.
2. **Pick the node.** `tree://node/{c}`; use the leaf node unless the user names one.
3. **Find the item id.** `tree://item/{c}/{n}` and walk `domain → kind → name`
   (e.g. `Go` → `functions` → `litar.Main`). Note its description; it comes from an `Index@/…` block.
4. **Get its blocks.** `blocks://refer/{c}/{n}/{item}`. Among them, the block whose code
   defines the item is the definition; the others are its users.
5. **Follow one level out.** For items the definition itself carries (callees, types, variables),
   fetch their `blocks://refer` too, but only as far as the explanation needs.
6. **Explain.** Answer, then support it.

For a chunk instead of an item: `tree://chunk` → `blocks://effective` (what produces it)
and `blocks://depend` (who uses it). For a file: `file://{c}/{n}/{path}`.

## Answer shape

- Open with one sentence: what the item is and does.
- Say where it lives: node, then chunk as `chunk@:label@:Module`, and how many blocks carry it.
- Show the defining code, then explain its steps in prose.
- Explain each user block: who calls or reads it and why.
- Point out design choices visible from the cross-references, such as a thin wrapper
  that keeps logic testable.
- Use litar terms (node, chunk, block, item) and copy item names exactly.
