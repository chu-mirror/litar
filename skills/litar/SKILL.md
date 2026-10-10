---
name: litar
description: "read and edit litar archive, interact with litar command line tool"
---

# Read and edit litar archive

A litar archive follows the syntax defined in `references/syntax.md`.
Follow the style guidance in `references/style.md`.

# Invoke command line tool

Evaluate an expression in an archive.

```bash
./litar -p "label@:module@/chunk@|filter" archive.la
```

Print the blocks that extend a chunk reference without evaluating them.
A specialized reference passes its extra labels
to included references that name no labels of their own.
Each module has a built-in `meta` chunk.

```bash
./litar -l "label@:module@/chunk" archive.la
```

