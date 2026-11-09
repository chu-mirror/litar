---
name: litar-cross-reference
description: Build cross-references between litar blocks by marking the non-local items they define or use (functions, types, variables, macros, options) with `@~` item references. Use when asked to add or improve cross-references, items, domains, or `@~` in a litar composition, or when writing new blocks that touch items shared with other blocks.
---

# Building cross-references between litar blocks

litar does not understand the text it manages; blocks are linked by listing the
items they define or use after `@~`. Every block carrying the same item is connected.

References:
- Item and meta-chunk syntax: Items and Meta chunks in [language.md](../references/language.md).
- What to mark and what not to: Cross-references in [style.md](../references/style.md).
- Exploring, applying, and verifying edits: the [litar-editing](../litar-editing/SKILL.md) skill.

## Workflow

```
- [ ] 1. Snapshot the outputs
- [ ] 2. Collect items per block
- [ ] 3. Filter out what must not be marked
- [ ] 4. Name items consistently
- [ ] 5. Add the item lists
- [ ] 6. Describe domains and key items
- [ ] 7. Verify
```

**1. Snapshot.** Evaluate every file of the node (`tree://file`, `file://`) and keep the outputs.
Items are not content, so these must not change.

**2. Collect.** From `blocks://node`, list for each block what its code defines or uses:
functions, data types, global or static variables, macros and constants,
command-line options, configuration keys, files, protocols.

**3. Filter.** Apply the cross-reference rules of the style reference:
keep only items whose context reaches beyond the block.

**4. Name.** `language@/kind@/name`, kinds as plural nouns, names exactly as in code:
- `C@/functions@/heap_alloc`, `C@/types@/struct context`, `C@/variables@/context_main`, `C@/macros@/NEW`;
- `Go@/functions@/greet.Hello`, `Go@/types@/greet.Options` (qualified by package);
- non-code domains when useful: `CLI@/options@/--no-log`.

Reuse the domains already in `tree://item`; never add a synonym for an existing kind.

**5. Add.** Mark the block that defines each item and every block that uses it.
Append the `@~` list after the content and any pipeline, changing nothing else.
Rewriting an existing `@+` block follows "Change or remove one `@+` block" in litar-editing.

**6. Describe.** Add an `Index@/…` block for each new domain, and one
for items whose meaning is not obvious from the name:

```
@<Index@/C@/functions@=
{"description": "Functions defined in this project."}
@
@<Index@/C@/variables@/context_main@=
{"description": "The context of the main thread; owns the main memory region."}
@
```

**7. Verify.**
- Every file evaluates exactly as in the snapshot.
- `tree://item` shows the intended domains, without near-duplicates.
- For a few items, `blocks://refer` lists the defining block and all users.
- `git diff` shows only `@~` lists and meta blocks (plus reordering).

## One-off chunks

A chunk that only wraps a well-defined unit (such as a function) can become an item
(see the example in the style reference). This removes a chunk name, so propose it to
the user before doing it.
