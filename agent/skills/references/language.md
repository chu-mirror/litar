# litar archive language

An archive (`.la`) is a list of expressions; bytes between expressions are comments.
A comment right before a block belongs to it.

## Controls

`@` plus an ASCII non-letter is a control; `@` plus a letter (`@word`) is text.
`@@` is a literal `@`, so text `@<x@>` is written `@@<x@@>`.
An expression ends at `@` followed by space, tab, CR, LF, or end of input.

## Names

- A name is trimmed at both ends; interior spaces stay (`@<Module 1@/my chunk@=`).
- Qualified name: `label2@:label1@:module@/submodule@/chunk`.
  Labels first (leftmost is most specific), then module path, then chunk.
  No `@/` means the root module.
- In block content, `.` / `..` start a module path relative to the block's header module.
  A header or filter name must not use them, so a `.` inside a name is an error.

## Blocks

```
Comment explaining a decision.
@<module@/chunk@=
content
@
```

- `@=` sets the chunk; `@+` extends it.
- Layout: one line break right after `@=` / `@+` and one right before the closing `@`
  (or `@|`, `@~`) are not content. To end content with a newline, leave an empty line before `@`.
  `@<msg@=Hello@` stores `Hello`.
- An `@+` block whose content ended with a line break is joined to the text before it
  with a line feed; otherwise it is concatenated.
- A reference in content: `@<chunk@>`, `@<label@:Module@/chunk@>`, `@<.@/sub@>`.
  A reference without labels inherits the labels of the reference being expanded.
- Pipeline: `@|filter1@|filter2` after the content filters this block's text;
  `@<chunk@|filter@>` filters one reference.

## Specialization

A block matches a reference when module and chunk are equal and the block's labels are
a suffix of the reference's labels. Matching blocks apply in node order
(base nodes first): `@=` replaces, `@+` appends. Those still contributing are the
**effective blocks**. `b@:a@:c` sees blocks for `c`, `a@:c`, and `b@:a@:c`, not `b@:c`.

## Files and file sets

```
@[hello.c@`sources@=
@<Language@/C@/Module@/module@>
@
```

- `@[path@=` regular file, `@(path@=` executable; `@+` is not allowed on files.
  A file's chunk is its path in the root module; it has no labels or module.
  `tree://chunk` lists it under `File`, one branch per directory (`File` → `cmd` → `main.go`).
  Move or remove files with the `operate_files` tool.
- `` @`set `` adds the file to a file set (repeatable).
- A filter sees no archive files unless given a view after its name:
  `@+set` add a set, `@-set` remove a set, `@,path` add a file, `@!path` remove a file.
  `@<@|list files@+sources@>` runs a filter with empty input and the view `sources`.

## Filters

A filter is a chunk whose content is a script starting with a shebang.
stdin is the text, stdout replaces it, a non-zero exit fails evaluation.
A failed filter's stdout is dropped; write what explains the failure to stderr.
Files in the view are visible, read-only, in the working directory.

## Items

```
@<chunk@=
int times;
@|filter
@~ Project@/hello@/C@/variables@/times @~ Project@/hello@/C@/functions@/main @
```

- `@~` at the top level of the body, after the pipeline, lists items; the end control closes the last.
- An item is a path of names, `owner@/kind@/name` by convention ([style.md](style.md)). Items are not content.
- Blocks carrying the same item are connected (`blocks://refer`).

## Meta chunks

Reserved modules hold JSON `{"description": "…"}` for descriptions shown in the trees:

| Chunk | Describes |
| --- | --- |
| `Chunk@/{chunk path}` | a module or chunk |
| `Specialization@/{label1}@/{label2}` | `label2@:label1` (labels reversed) |
| `File@/{dir}@/{file}` | a file or directory (path parts separated by `@/`) |
| `Index@/{item path}` | an item domain or item |

Describe a new module, domain, or non-obvious item when you create it:

```
@<Index@/Project@/heap@/C@/variables@/context_main@=
{"description": "The context of the main thread; owns the main memory region."}
@
```

## Compositions

A composition is a directory with `composer.json`:
`description`, `ground` (another composition's name, or null), and `nodes`.
Each node has `name`, `archive` (relative path), `description`, `mappings`, and `base`
(the node names from the root to its parent; `[]` for the root).
A mapping (`path`, `origin`, `fileset`) shows a file or directory outside the archives in the node.
A node's blocks are its base's blocks followed by its own archive's.
A ground's nodes come first; grounds are searched in the current directory,
then `$LITAR_LIBRARY`.
