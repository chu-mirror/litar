---
name: litar-split
description: Divide long blocks of a litar composition into chunks of about 150 tokens following the style guide — outline trees, order-irrelevant sequences, declarations before quoting chunks — editing the code where the split needs it, rebuilding the node in place through the litar MCP server, and verifying the result with a diff review, a build, and the tests. Use when asked to split, divide, or shorten long blocks, or to restructure chunks of a litar archive according to design/style.md.
---

# Splitting long litar blocks

Rules: Blocks and Naming chunks in [style.md](../references/style.md) (the full text is
`design/style.md` in the litar repository). Syntax: [language.md](../references/language.md).

## Scripts

Run them; do not rewrite them. They need `LITAR` (the binary) and `LITAR_LIBRARY` (the grounds),
e.g. `LITAR=bin/litar LITAR_LIBRARY=.` in the litar repository.

- `scripts/measure.py [-n NODE] [-l LIMIT] COMPOSITION_DIR` lists the node's own blocks longer
  than LIMIT tokens (default 150; estimated as characters / 4), longest first.
- `scripts/rebuild.py [-n NODE] [-o OUT] [--files DIR] [--same] [--dry-run] COMPOSITION_DIR PATCH...`
  deletes and re-creates the leaf node, re-applying every block in its original order with each
  patched block replaced in place by its group. It writes `OUT/changes.diff`, the diff of every
  evaluated file that changed; `--files DIR` writes all evaluated files to build and test.
  `--same` fails if any file changed; `--dry-run` only checks that the patches match.

## Workflow

```
- [ ] 1. Measure and choose a pilot
- [ ] 2. Write the patch
- [ ] 3. Dry-run, then rebuild a copy
- [ ] 4. Rebuild the composition
- [ ] 5. Review with the user, then continue
```

**1. Measure.** Run `measure.py`. Leave whole: generated or data blocks (go.sum, requirement lists)
and order-dependent structures (if-else chains, switches with prefix tests, case tables).
Start with one package or file as a pilot so the user can review names and shape.

**2. Patch.** Read the blocks from `blocks://node`. Write a patch file in a scratch directory;
groups are separated by lines of `%%`:

```
Optional new comment for the outline block.
@<cli@:litar@:Language@/Go@/File@/functions@+
func run(args []string) int {
prog := "litar"
@<Project@/litar@/print the usage of 'prog' if asked@>

@<Project@/litar@/parse the options@>
}

@~ Project@/litar@/Go@/functions@/litar.run @

@<Project@/litar@/print the usage of 'prog' if asked@=
...
@~ Project@/litar@/Go@/functions@/litar.usage @
%%
@<cli@:litar@:Language@/Go@/File@/functions@+
func dispatch(mode string) (string, error) {
...
```

The first block of a group must keep the header and first content line of the block it replaces.
Follow it with the sub-chunk blocks in reading order (depth first). Shape the split:

- Make the old block an outline tree: its steps become references, each on a line of its own.
- Put the sub-chunks in the project's module (`Project@/litar@/…`), or in a submodule of the module that
  owns the outline when nothing outside it may refer to them; refer to them relatively from
  blocks in that module (`@<.@/…@>`).
- Name each step after its action, quoting the objects it works on. Declare every quoted object
  before the reference that quotes it, in the parent block.
- Never use an object outside the chunk that declares it. Move the declaration up into the block
  whose expansion holds every use, or keep a closure's declaration line in the parent and make
  only its body a chunk.
- Split a switch with distinct constant cases into a sequence (`cases of 'opt' in 'run'`):
  start with `@=`, add the rest with `@+`.
- Put a comment before each block that carries a decision. Mark each block with the items
  it uses (`@~`), dropping them from the outline when they move away.
- Edit the code where the split needs it. Move a declaration, turn a closure into a function,
  extract a helper, merge repeated cases, or rename a local so the chunk names read well.
  Keep the behavior; leave unrelated improvements for a separate task.
- When the split is purely structural, keep the statements and blank lines and pass `--same`.

Layout facts:

- Expansion does not re-indent. Grounds that format code (gofmt, clang-format) fix the
  indentation, so sub-chunks can be written unindented. Without a formatter, give the sub-chunk
  its absolute indentation and put the reference at the start of its line.
- End a tree sub-chunk without an empty line before `@`. Otherwise a blank line appears
  after the reference.
- Sequence blocks without a trailing empty line are joined by one line feed.
  With the empty line, a blank line separates them.

**3. Check.** Run `rebuild.py --dry-run`. Then rebuild a copy of the composition directory
(composer.json plus the archive) with `--files DIR`:

- read `OUT/after.la` around the change;
- read `OUT/changes.diff`; every hunk must be an intended edit;
- build the files in DIR and run the project's tests there (restore executable bits first).
  If tests need the surrounding repository, use the project's own build or sync target
  on a clean working copy instead. A ground's check chunk, if it has one, can replace the build.

Fix the patch and repeat until all three pass.

**4. Rebuild.** The archive must be committed, so that a failed run can be restored.
Run `rebuild.py` on the composition and compare `changes.diff` with the one from the copy.
`git diff` must touch only the patched region, and composer.json must be unchanged.
A running litar MCP server in the editor is now stale; ask the user to restart it.

**5. Review.** Report the new chunk names and sizes, and every code change with its reason. To revise a split, restore the archive from
version control and rebuild with the edited patch: `rebuild.py` refuses a patch whose
sub-chunks are already in the node. Keep the patch files until the work is done.

## Pitfalls

- Do not `delete_chunk` a shared chunk such as `functions` and re-apply its blocks:
  compaction appends them at the end of the archive, away from their packages.
- `apply_block` appends text as is, and an archive ending with `@` turns a following `@<` into `@@<`.
  `rebuild.py` keeps each block's leading line break.
- `file://` resources take the file id from `tree://file`, not its path.
