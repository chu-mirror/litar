---
name: litar-consult-chunk
description: "Summarize a specified litar chunk reference. Check the blocks that depend on it and how they use it, check every effective block applied to it, and explain how the reference implements its functionality. Use when the user runs /litar-consult-chunk, or asks to summarize, consult, or explain a chunk reference such as label@:module@/chunk."
---

# Consult a chunk

Summarize one chunk reference. Do not edit the archive.

The user supplies an archive path and a chunk reference.
The reference has no closing `@`.
A literal `@` in a name is written `@@`.
Read `doc/syntax.md` when the form is unclear.
If the archive or the reference is missing, ask before calling a tool.

Use the litar MCP tools `blocks_depending`, `blocks_effective`, `list_modules`, and `evaluate_expression`.
Pass the archive path and the reference exactly.

## Steps

1. Call `blocks_depending`.
   The list is the blocks whose content or filters name this reference or a specialization of it.
   It is not transitive.
   An empty list means no block in the archive names it.
2. For each of those blocks, say how it uses the reference.
   Use its comment, content, and filters.
   A use is an embedded reference, a filter name, or both.
   An unlabeled reference takes the labels of the block that contains it.
   A reference with no module stays in the module selected where that block was written.
   Quote only the line or lines that contain the use.
3. Call `blocks_effective`.
   These are the blocks whose text remains after the chain is applied.
   A later `@=` drops every earlier block on that chain.
   `@+` appends.
   An undefined reference is an error from this call.
   Report that, and still report the dependers from step 1.
4. Explain how the reference implements the behavior its uses require.
   The dependers say what is required.
   The effective blocks, in order, are the implementation.
   When a block's role is itself a reference, call `blocks_effective` on that inner reference.
   Stop at a cycle, and stop once that inner reference's own text states the behavior.
   Call `list_modules` when the module summary is needed to name the intended role.
   Call `evaluate_expression` when the expanded text is needed to support a claim.
   Summarize a long expansion.
   Do not paste a whole program.

## Reply

Lead with what the reference does.
Then list each depender, where it is, and how it uses the reference.
Then list each effective block: its operator, file, line range, and what it contributes.
Then explain how those blocks produce the behavior.
Name a file and line range when pointing at a block.
