---
name: litar-task
description: Do a piece of work in a litar composition — a feature, fix, test, or restructuring — by applying blocks through the litar MCP server in a task node, then digesting the node by moving each block to the node and module where it belongs. Use when editing a litar composition (.la archives, chunks, blocks, items), when starting a task in one, or when the user asks to digest, clean up, or fold a task node into its base.
---

# Litar tasks

Tools, refs, and edit mechanics: [mcp.md](../references/mcp.md).
Syntax: [language.md](../references/language.md).
Naming, placement, block size, items: [style.md](../references/style.md).

## Lifecycle

```
- [ ] 1. Describe the task
- [ ] 2. Create the task node
- [ ] 3. Do the work in the task node
- [ ] 4. Digest
- [ ] 5. Check the other nodes
```

A small change with an obvious place (a fix inside one block, a renamed chunk) needs no task node:
edit the owning node directly and go to step 5.

**1. Describe.** One or two sentences: what is being done and which node it builds on.
Use it as the task node's description.

**2. Create.** `create_node` with `base` = the node it builds on and a new archive path
next to the composer file. If that base comes from a ground, the task node still goes in
the current composition.

**3. Work.** Apply every new block to the task node, following the style reference:

- find the context first: `blocks://effective` and `blocks://depend` of the chunk,
  `blocks://refer` of the items involved;
- change an existing block with `apply_block` and `overrides`; a block of a base node is
  changed in that node;
- put a comment before each block that carries a decision;
- mark the items each block defines or uses (`@~`), reusing the domains in `tree://item`;
- evaluate the outputs and build or test them.

**4. Digest.** See below.

**5. Check.** Evaluate and build the outputs of every node under each node that received
blocks, not only the task node.

## Digest

### a. Snapshot

Evaluate every file (`tree://file`, `file://`) of the task node, and of every other node
under each candidate base. Keep the outputs in a temporary directory.

### b. Decide where each block belongs

Read `blocks://node` of the task node. A block has two places to settle:

- **Its module.** Does the chunk improve something shared (`Tool`, `Language@/…`, `UI@/…`,
  `Framework@/…`) or does it belong to the project (`Project@/…`)? If it was written under
  the wrong module, `rename` the chunk in the task node first.
- **Its node.** The node that owns that module: a block of a shared module goes to the base
  that holds it; a block of the task's own project stays, or goes to the project's node
  when the task node is only temporary.

Ask the user when the target is unclear.

A block may move only if it still works in the target:
- every chunk it references must be defined in the target's chain. A missing reference
  silently expands to empty, so check each `@<…@>` and filter name;
- moving changes order: the base's blocks precede the task node's. A moved `@+` must not rely
  on a task-node `@=` before it, and a task-node `@=` will override a moved block of the
  same reference;
- blocks of one chunk that depend on each other's order move together.

### c. Move

- A target in the same composition: `move_block` each block, in order.
- A target in a ground: import the ground composition, `apply_block` each block there
  with its comment, then `delete_block` it from the task node.
- `delete_node` the task node when nothing is left in it.

### d. Verify

- The task node's outputs (if it still exists) equal the snapshot.
- Other nodes under the targets changed only where the moved blocks were meant to change them;
  build and run their generated programs.
- `git diff` shows the moved blocks in the target archives.

## Pitfalls

- A running litar server does not see archives changed outside it (a script, version control,
  a rebuilt binary). Call `import_composition` again, or ask the user to restart the server.
- If a change cannot be expressed through the tools, ask the user before editing a file.
