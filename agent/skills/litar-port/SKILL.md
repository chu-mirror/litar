---
name: litar-port
description: Port an existing project (source tree, scripts, build files) into a litar composition in stages — first every file as one exact block, then split into structured chunks on a matching ground, then cross-referenced — checking after each step that the evaluated files still match the original project byte for byte. Use when the user asks to port, convert, import, or migrate a project or codebase into litar form.
---

# Porting a project to litar

Tools and edit mechanics: the [litar-editing](../litar-editing/SKILL.md) skill.
Archive syntax: [language.md](../references/language.md). Chunk and block rules: [style.md](../references/style.md).

## Scripts

Run them; do not rewrite them.

- `scripts/file-block.sh [-C PROJECT] [-s SET]... PATH...` prints one exact file block per path
  (`@(` for executables, `@[` otherwise), escaping `@` and handling the layout line breaks.
  It skips binary files and files ending in a lone CR, with a warning.
- `scripts/port-check.sh [-v] [-o OUT] ARCHIVE PROJECT [PATH...]` evaluates each path with
  `litar -f ARCHIVE -p PATH` and compares it with `PROJECT/PATH`
  (default paths: `git ls-files`). `-v` shows diffs; `-o OUT` writes the evaluated tree
  for building. Needs `LITAR` (the binary) and `LITAR_LIBRARY` (the grounds).

## Workflow

```
- [ ] 0. Survey the project
- [ ] 1. Set up the composition
- [ ] 2. Stage 1: files verbatim
- [ ] 3. Stage 2: structure, one file at a time
- [ ] 4. Stage 3: cross-references
- [ ] 5. Build and test from litar
```

**0. Survey.** List the files (`git ls-files`), languages, and the build and test commands.
Separate the files to port (text source, scripts, build files, docs) from those that stay
outside (binary assets, generated files, vendored code). Group the files into file sets
by role, e.g. `sources`, `tests`, `build`.

**1. Set up.** Ask the user where the composition goes. Ground on `standard`, the standard library
of litar: import it from `LITAR_LIBRARY` and read `tree://chunk` of its node Standard
(`Language@/Go@/File` and `Language@/Go@/Module` hold the Go structures, `Language@/C@/Module`
the C ones, `Tool` the shared filters, `UI@/Command Line` the command line generators).
Use `create_composition` with that ground, then `create_node` for the project with `base` = Standard.
Put the project's own chunks under `Project@/<project>`.

**2. Stage 1: verbatim.** For each file, or a few at a time:

```bash
scripts/file-block.sh -C "$PROJECT" -s sources src/main.c > /tmp/block.la
```

Apply the output with `apply_block`. For a large project, ask the user whether the script
output may be appended directly to the new, still empty archive instead; re-import afterwards.
Then:

```bash
LITAR=bin/litar LITAR_LIBRARY=. scripts/port-check.sh path/to/project.la "$PROJECT"
```

Do not continue until it reports `different 0`, apart from files that stay outside.

**3. Stage 2: structure.** Take one file at a time, starting with the most central:

1. Read how the ground builds that kind of file. Find its file or module chunk in
   `tree://chunk` of the node Standard, then read its blocks.
2. Re-apply the file block as a tree that references the ground's structure,
   for example `@<prog@:executable@:Language@/C@/Module@/module@>` or `@<cmd@:command@:Language@/Go@/File@/file@>`.
3. Move the code into chunks that extend the ground's sequences (includes, types, functions)
   and into named trees for big bodies, following the style rules. Name the trees in a module
   named after the project (`Project@/…`), leaving the root module to the file chunks.
4. Add a comment before each block that carries a design decision visible in the code.
5. Run `port-check.sh` on that file.
   - If a ground formatting filter changes the layout, compare against the original run
     through the same formatter, and confirm with a build instead.
   - A difference in content means the structure is wrong; fix it before the next file.

Turn information repeated across files (CLI options in help text, parser, and manpage)
into one declarative chunk with filters.

**4. Stage 3: cross-references.** Follow the [litar-cross-reference](../litar-cross-reference/SKILL.md) skill.

**5. Build and test.** `port-check.sh -o /tmp/port-out ARCHIVE PROJECT`, then run the project's
build and tests inside `/tmp/port-out`. Report any files left outside the composition.

## Stopping points

Each stage is a usable result: stage 1 is an exact copy, and stage 2 can stop after any file.
After each stage, tell the user what has been done and which files remain.
