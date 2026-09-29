---
name: litar
description: "interact with litar, print chunk, author an litar archive"
---

litar implements `reference/syntax.md`.

## Run

Print the contents of a chunk in an archive.

```bash
./litar -p CHUNK archive.la
```

The expanded chunk goes to stdout. The arguments are the flag `-p`,
the chunk name, and the archive path, in that order.

## Author an archive

Follow the style guidance in `reference/style.md`.

