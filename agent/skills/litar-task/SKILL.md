---
name: litar-task
description: Run a litar task in its own node and digest it afterwards — create a task node on the node it builds on, apply the work there, then move blocks that improve a base into that base node, rebuild the task node from what remains (or delete it), and check every affected node. Use when starting a feature, fix, or test in a litar composition, or when the user asks to digest, clean up, or fold a task node into its base.
---

# Litar task nodes and digest

Edit mechanics (ids, applying, rewriting `@+` blocks, verifying) are in the
[litar-editing](../litar-editing/SKILL.md) skill; naming and block rules in
[style.md](../references/style.md).

## Lifecycle

```
- [ ] 1. Describe the task
- [ ] 2. Create the task node
- [ ] 3. Do the work in the task node
- [ ] 4. Digest
- [ ] 5. Check the other nodes
```

**1. Describe.** One or two sentences: what is being done (a feature, a framework improvement,
test cases) and which node it builds on. Use it as the task node's description.

**2. Create.** `create_node` with `base` = the node it builds on and a new archive path
next to the composer file. If that base comes from a ground, the task node still goes in
the current composition.

**3. Work.** Apply every new block to the task node only, until the task is complete
and its outputs are verified.

**4. Digest.** See below.

**5. Check.** Evaluate and build the outputs of every node under each base that received
blocks, not only the task node.

## Digest

### a. Snapshot

Evaluate every file (`tree://file`, `file://`) of the task node, and of every other node
under each candidate base. Keep the outputs.

### b. Classify each own block of the task node

Read `blocks://node` of the task node. For each block, decide the target:

- **Base node:** the block improves what the base provides. For example: a generic filter,
  a reusable structure chunk, a fix to a framework chunk, a meta description of a base chunk.
  Pick the lowest node where it makes sense (e.g. common filters belong under `Tool` in the ground `standard`).
- **Stays:** the block is specific to this task (the application, its files, its tests).

Ask the user when the target is unclear.

A block may move only if it still works in the base:
- every chunk it references must be defined in the base's chain. A missing reference
  silently expands to empty, so check each `@<…@>` and filter name;
- moving changes order: the base's blocks precede the task node's. A moved `@+` must not rely
  on a task-node `@=` before it, and a task-node `@=` will now override a moved block of the
  same reference;
- blocks of one chunk that depend on each other's order move together.

### c. Move

1. Import the composition that declares the base node if it is a ground node.
2. Apply each moving block to the base node, with its comment, unchanged.

### d. Rebuild the task node

A digest replaces the task node instead of deleting blocks one by one:

1. Save the text (comment and block) of every block that stays, in order.
2. If the task node has child nodes, stop: `delete_node` would delete them too.
   Remove the moved blocks from the task node with the `@+` rewrite in litar-editing instead.
3. `delete_node` the task node.
4. If blocks stay, `create_node` again with the same name, description, base, and archive path,
   then apply the saved blocks in order. If none stay, do not recreate it.

### e. Verify

- The task node's outputs (if it still exists) equal the snapshot.
- Other nodes under the bases changed only where the moved blocks were meant to change them;
  build and run their generated programs.
- `git diff` shows the moved blocks in the base archives and the remaining ones in the task archive.
