---
name: litar-port
description: Port an existing project (source tree, scripts, build files) into a litar composition in stages — first every file as one exact block, then divided into structured chunks on the standard ground, then cross-referenced, then given a build composition — checking after each step that the evaluated files still match the original project. Use when the user asks to port, convert, import, or migrate a project or codebase into litar form, or to divide long blocks or add cross-references in a ported one.
---

# Porting a project to litar

Tools, refs, and edit mechanics: [mcp.md](../references/mcp.md).
Syntax: [language.md](../references/language.md).
Naming, placement, block size, items: [style.md](../references/style.md).

## Scripts

Run them; do not rewrite them. They need `LITAR` (the binary) and `LITAR_LIBRARY` (the grounds).

- `scripts/file-block.sh [-C PROJECT] [-s SET]... PATH...` prints one exact file block per path
  (`@(` for executables, `@[` otherwise), escaping `@` and handling the layout line breaks.
  It skips binary files and files ending in a lone CR, with a warning.
- `scripts/port-check.sh [-v] [-o OUT] ARCHIVE PROJECT [PATH...]` evaluates each path and
  compares it with `PROJECT/PATH` (default paths: `git ls-files`). `-v` shows diffs;
  `-o OUT` writes the evaluated tree for building.
- `scripts/measure.py [-n NODE] [-l LIMIT] COMPOSITION_DIR` lists the node's own blocks whose
  content is longer than LIMIT tokens (default 150), longest first, each with its ref.

## Workflow

```
- [ ] 0. Survey the project
- [ ] 1. Set up the composition
- [ ] 2. Stage 1: files verbatim
- [ ] 3. Stage 2: structure, one file at a time
- [ ] 4. Stage 3: cross-references
- [ ] 5. Stage 4: build composition
```

**0. Survey.** List the files (`git ls-files`), languages, and the build and test commands.
Separate the files to port (text source, scripts, build files, docs) from those that stay
outside (binary assets, generated files, vendored code); those can be mapped in with `map_file`.

**1. Set up.** Ask the user where the composition goes. Ground on `standard`, the standard library
of litar: import it from `LITAR_LIBRARY` and read `tree://chunk` of its node Standard
(`Language@/Go@/File` and `Language@/Go@/Module` hold the Go structures, `Language@/C@/Module`
the C ones, `Tool` the shared filters, `UI@/Command Line` the command line generators).
Use `create_composition` with that ground, then `create_node` for the project with `base` = Standard.
Put the project's own chunks under `Project@/<project>`.
Take the file sets from the filters of the ground you will use (the Go module filters expect
`go module` and `go tests`); otherwise group files by role.

**2. Stage 1: verbatim.** For each file, or a few at a time:

```bash
scripts/file-block.sh -C "$PROJECT" -s sources src/main.c > /tmp/block.la
```

Apply the output with `apply_block`. For a large project, ask the user whether the script
output may be appended directly to the new, still empty archive instead; import it again afterwards.
Then:

```bash
scripts/port-check.sh path/to/project.la "$PROJECT"
```

Do not continue until it reports `different 0`, apart from files that stay outside.

**3. Stage 2: structure.** Take one file at a time, starting with the most central:

1. Read how the ground builds that kind of file: find its file or module chunk in
   `tree://chunk` of the node Standard, then read `blocks://effective` of it.
2. Override the file block with a tree that references the ground's structure,
   for example `@<prog@:executable@:Language@/C@/Module@/module@>` or
   `@<cmd@:command@:Language@/Go@/File@/file@>`.
3. Move the code into blocks that extend the ground's sequences (includes, types, functions),
   one well-defined unit per block.
4. Run `measure.py` and divide the long blocks as the style reference says
   (Dividing a long block), overriding each with its outline and applying the sub-chunks.
   Keep the behavior; leave unrelated improvements for a separate task.
5. Add a comment before each block that carries a design decision visible in the code.
6. Run `port-check.sh` on that file.
   - If a ground formatting filter changes the layout, compare against the original run
     through the same formatter, and confirm with a build instead.
   - A difference in content means the structure is wrong; fix it before the next file.

Turn information repeated across files (CLI options in help text, parser, and manpage)
into one declarative chunk with filters.
Restructure with `rename` and `operate_files`, not by applying blocks again under new names.

**4. Stage 3: cross-references.** Items are not content, so no output may change.

1. For each block, list what its code defines or uses beyond the block:
   functions, types, global variables, macros, options, configuration keys.
2. Name the items as the style reference says (Items), reusing the domains in `tree://item`.
3. Override each block with its `@~` list added after the content and any pipeline,
   on the defining block and on every user.
4. Add an `Index@/…` description for each new domain and each item whose name does not explain it.
5. Check: `port-check.sh` still passes, and `blocks://refer` of a few items lists
   the definition and all users.

**5. Stage 4: build.** Create a second composition grounded on the project's,
with one node holding `Project@/<project>@/Build`: the commands that write the files out,
build, and test, and mappings for the directories they need.
Until it exists, `port-check.sh -o /tmp/port-out ARCHIVE PROJECT` writes the tree;
run the project's build and tests there. Report any files left outside the composition.

## Stopping points

Each stage is a usable result: stage 1 is an exact copy, and stages 2 and 3 can stop after any file.
After each stage, tell the user what has been done and which files remain.
