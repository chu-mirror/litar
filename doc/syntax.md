# Syntax

This document defines the syntax of litar archives (`.la`), the `composer.json` file,
and command-line expressions.
See [implementation.md](implementation.md) for the command-line options, evaluation rules,
and MCP server.

# Scan

An archive is a sequence of bytes scanned one byte at a time.
It consists of **expressions** (blocks and files) and **comments** (all bytes outside expressions).
Bytes at or above 128 are ordinary text.

An `@` introduces a control sequence depending on the byte that follows it:

- **Control (`@` + ASCII non-letter):** When `@` is followed by an ASCII byte outside
  `A`–`Z` and `a`–`z`, those two bytes form a control.
- **Escape (`@@`):** Two `@` bytes represent a single literal `@`. An escape neither starts
  nor ends an expression; inside a name or body it decodes to one `@`. To write the literal
  text `@<kept@>`, write `@@<kept@@>`.
- **Ordinary text (`@` + letter or non-ASCII):** When `@` is followed by an ASCII letter
  (`A`–`Z`, `a`–`z`) or a byte at or above 128, both bytes stay as ordinary text.
  `@word` needs no escape.
- **End control (`@` + whitespace or EOF):** `@` followed by a single space, tab,
  carriage return (`\r`), or line feed (`\n`)—or a final `@` at the end of the input—closes
  the current expression. The `@` and that single whitespace byte are consumed as the delimiter
  and are not part of the preceding name or content. When `@` is followed by `\r\n`, `@\r` is
  the end control and `\n` remains outside the expression.

All controls are listed below:

| Control | Meaning |
| --- | --- |
| `@@` | literal `@` |
| `@` + space, tab, `\r`, `\n`, or EOF | end the current expression |
| `@<` | start a block header, or start a chunk reference in a body |
| `@>` | end a chunk reference in a body |
| `@=` | set (replace) a chunk or define a file |
| `@+` | extend (append to) a chunk, or add a file set in a view |
| `@-` | subtract a file set in a view |
| `@/` | separate module path components, separate module from chunk, or separate item path components |
| `@:` | attach a specialization label |
| <code>@&#124;</code> | start a filter pipeline or separate filters |
| `@~` | attach an item reference to a block |
| `` @` `` | declare file-set membership on a file |
| `@,` | add a single file in a view |
| `@!` | remove a single file in a view |

At the top level of an archive:

- `@<` starts a block expression;
- `@[` starts a regular file expression;
- `@(` starts an executable file expression;
- `@@` is literal `@` in a comment.

Any other control at the top level (such as a bare `@|`, `@~`, or end control) is an error.
To write `@<`, `@[`, or `@(` inside a comment, escape the `@` (for example, `@@<`).

# Names

A **name** is a span of bytes running from the current scan position up to the next control:

1. Leading and trailing spaces, tabs, carriage returns, and line feeds are trimmed.
2. Each `@@` inside the trimmed span is decoded to a single `@`.
3. Interior whitespace—including spaces and line breaks between words—is preserved.

For example, `@< Module 1 @/ chunk @=` names the chunk `chunk` in the module `Module 1`,
and `@< @@ @=` names the chunk `@`.

A name that is empty after trimming is an **empty name**.
Empty names are an error for labels, module components, chunk names, filter names, file paths,
file-set names, and item components, with two exceptions:

- a leading `@/` in a qualified name, which explicitly names the root module;
- an omitted chunk name in a filtered reference (`@<@|filter@>`), which passes empty input
  to the pipeline.

# Qualified name

A **qualified name** identifies a chunk, optionally specialized by labels and placed in a module:

```
label2@:label1@:module@/submodule@/chunk
```

It has three parts, in order:

1. **Labels (optional):** Zero or more names, each followed by `@:`. The leftmost label is
   the most specific specialization. Labels must come before the module path; a `@:` after
   `@/` is an error.
2. **Module path (optional):** Zero or more module names separated by `@/`, followed by `@/`.
3. **Chunk name:** The final name.

Modules form a directory-like tree in which modules are directories and chunks are files:

- The root module is written `/`.
- Omitting `@/` (as in `chunk` or `label@:chunk`) places the chunk in the root module.
- A leading `@/` (as in `@/chunk` or `label@:@/module@/chunk`) also names the root module
  explicitly. Any other empty module component is an error.

Inside a block's body content, a reference's module path may use `.` and `..`:

- A path starting with `.` or `..` resolves relative to the module of the enclosing block's
  header: `.` is that module and `..` is its parent (`..` at the root stays at the root).
- Using `.` or `..` in a block header, a filter name, or a command-line argument is an error.

# Block

A block sets (`@=`) or extends (`@+`) a chunk:

```
@<qualified name@=
content
@
```

The header is `@<`, a qualified name, and `@=` or `@+`.
A block header cannot contain a filter pipeline (`@|` in the header is an error).
The closing end control (`@` followed by whitespace or end of input) is required;
reaching the end of input without it is an unterminated-block error.

## Body layout and line breaks

The body begins immediately after `@=` or `@+`:

- **Leading line break:** At most one `\n`, `\r\n`, or `\r` right after `@=` or `@+` is
  stripped as layout, not content. (`\r` is stripped only when it comes first, so `\n\r`
  strips only `\n`.)
- **Trailing line break:** At most one `\n` or `\r\n` at the end of the content—immediately
  before the closing `@`, `@|`, or `@~`—is also stripped as layout, and records that the
  block's content ended with a line break.

Because one leading and one trailing line break are stripped, a standard multi-line block
stores its content without a trailing newline:

```
@<msg@=
Hello
@
```

This block stores `Hello` (and is marked as ending with a line break).
To include a trailing newline in the stored content, leave an empty line before the closing `@`:

```
@<msg@=
Hello

@
```

A single-line block has neither a leading nor a trailing line break, so it stores `Hello` directly:

```
@<msg@=Hello@
```

## References in a body

The content of a block may contain ordinary text, `@@` escapes, and chunk references:

```
@<qualified name@>
@<qualified name@|filter1@|filter2@>
```

A reference in a body starts with `@<`, contains a qualified name and an optional filter
pipeline, and ends with `@>`.
A `@|` between `@<` and `@>` belongs to that reference, not to the enclosing block.
References do not nest inside one another, and an end control before `@>` is an
unterminated-block error.
To write literal `@<` in content, write `@@<`.

## Block pipeline and items

After the content, a block body may optionally include a filter pipeline and item references
before the closing end control:

```
@<chunk@=
int times;
@|filter
@~ C@/variables@/times @
```

- **Filter pipeline (`@|`):** `@|` at the top level of the body ends the content and starts
  a filter pipeline that transforms this block's expanded content.
- **Item references (`@~`):** `@~` at the top level of the body (after the filter pipeline,
  if any) ends the content or pipeline and attaches an item reference to the block.

An **item** is a path of one or more names separated by `@/`:

```
domain@/subdomain@/name
```

The first name is the domain, and each subsequent name classifies the item further;
there is no fixed depth.
Multiple items can be attached to the same block by chaining `@~`:

```
@~C@/variables@/times@~C@/functions@/main@
```

The block's end control closes the last item.
Items are metadata used for cross-referencing (`-r` and `blocks://refer`); they are not part
of the block's content and are not printed when the block is evaluated.
To write literal `@~` in content, write `@@~`.

# Specialization

A chunk reference is a qualified name.
Applying a block to one specialization of a chunk does not affect unrelated specializations
of the same chunk.

The **predecessors** of a chunk reference are formed by removing labels one by one from the
left (from most specific to least specific).
For example, the predecessors of `spec2@:spec1@:module@/chunk` are:

1. `spec1@:module@/chunk`
2. `module@/chunk`

Note that `spec2@:module@/chunk` is **not** a predecessor of `spec2@:spec1@:module@/chunk`;
it is a separate specialization of `module@/chunk`.

## Matching and accumulation

A block **matches** a chunk reference when:

- their module and chunk name are equal, and
- the block's labels are a suffix of the reference's labels (an unlabelled block matches
  every specialization).

When a chunk reference is evaluated, all matching blocks in the node chain (the base nodes'
blocks in order, followed by the current node's archive) are applied in order:

- `@=` replaces the text accumulated so far with the block's expanded text.
- `@+` appends the block's expanded text. If the `@+` block's content ended with a layout
  line break and the accumulated text is not empty, a `\n` joins them; otherwise the text
  is appended directly.

The matching blocks that still contribute to the final text (from the last `@=` onward, or
all matching `@+` blocks if there is no `@=`) are the **effective blocks** of that chunk
reference.

## Label inheritance

When a reference `@<...@>` inside a block body omits labels, it inherits the labels of the
chunk reference currently being expanded.
If the inner reference writes its own labels, those labels are used as written instead.

# Filter pipeline

A **filter** is a chunk whose evaluated content is a script starting with a `#!` shebang line.
In a pipeline `@|filter1@|filter2`, filters run left to right: each filter receives the text
on standard input and its standard output replaces the text.

Each filter in a pipeline is a qualified name (optional labels, optional module without `.`
or `..`, and a chunk name), optionally followed by a file-set view.
A filter cannot have a pipeline of its own.
If a filter omits labels, it inherits the labels of the chunk reference it transforms.

Pipelines may appear:

- inside a body reference: `@<chunk@|one@|two@>`
- at the end of a block or file body: `@|one@|two@`
- in a print expression (`-p` or `evaluate_expression`).

Chunk references passed to `-l`, `-e`, or `-d` cannot have a pipeline.

# File

`@[` defines a regular file and `@(` defines an executable file:

```
@[hello.c@`source files@`executables@=
content
@

@(hello.sh@=
#!/bin/sh
echo "Hello"
@
```

- The file path is a single name, not a qualified name. Ordinary slashes (`/`) are allowed
  in the path; `@/` is a control and cannot appear in a path.
- A file has no labels and no module (`@:` and `@/` in a file header are errors), and must
  use `@=` (`@+` is an error).
- Zero or more `` @`name `` clauses after the path add the file to the named file sets.
- The body follows the same rules as a block body, including optional `@|` filters and
  `@~` items.

A file also defines a chunk in the root module whose chunk name is the file path.

# File set

By default, a filter invocation sees no files from the composition in its working directory.
A **view** is a file-set expression attached to a filter that specifies which files are
mounted for that filter.

A file-set expression is evaluated left to right starting from the empty set:

| Operation | Operand | Effect |
| --- | --- | --- |
| `@+` | file-set name | add all files in that file set |
| `@-` | file-set name | remove all files in that file set |
| `@,` | file path | add that single file |
| `@!` | file path | remove that single file |

For example, `@+file set1@-file set2@,file1@,file2@!file3` selects every file in `file set1`
that is not in `file set2`, adds `file1` and `file2`, and removes `file3`.

In a pipeline, a filter's view appears immediately after the filter's chunk name (before the
next `@|`, `@>`, end control, or end of expression).
Each filter in a pipeline has its own view.

A reference that has a filter pipeline may omit the chunk name:

```
@<@|list files@+source files@>
```

The omitted chunk supplies empty input (`""`) to the first filter in the pipeline.

# Reserved namespaces

Four top-level module names are reserved for metadata chunks:
`Chunk`, `Specialization`, `File`, and `Index`.
The remaining module components and the chunk name identify the object being described:

| Reserved chunk | Object described | Example |
| --- | --- | --- |
| `Chunk@/{chunk path}` | the module or chunk at `{chunk path}` | `Chunk@/Basic`, `Chunk@/Basic@/evaluate`, `Chunk@/main` |
| `Specialization@/{label1}@/...` | specialization `...@:label1` (labels reversed) | `Specialization@/label1@/label2` (for `label2@:label1`) |
| `File@/{file path}` | the file or directory at `{file path}`, its parts separated by `@/` | `File@/cmd`, `File@/cmd@/main.go` |
| `Index@/{item path}` | the item or domain at `{item path}` | `Index@/C@/variables`, `Index@/C@/variables@/times` |

A module and a chunk with the same path share one description, and so do a domain and an item.

Each metadata chunk evaluates to a JSON object with a `description` field:

```json
{"description": "<description of that module, chunk, specialization, file, directory, domain, or item>"}
```

These chunks are omitted from the chunk tree; their descriptions populate the chunk,
specialization, file, and item trees.
The chunk tree lists a file's chunk under `File`, one branch for each directory of its path,
rather than as a root-module chunk named by the path.

# Composer file

A **composition** is a directory containing `composer.json`, a JSON object with
`description`, `ground`, and `nodes`:

```json
{
    "description": "An example of composition",
    "ground": null,
    "nodes": [
        {
            "name": "Basic C Structure",
            "archive": "c-structure.la",
            "description": "Build common patterns that appear in C programs.",
            "mappings": [],
            "base": []
        },
        {
            "name": "Program Hello",
            "archive": "programs/hello.la",
            "description": "Write the classic example in the first lesson of programming.",
            "mappings": [
                {
                    "path": "data/samples.txt",
                    "fileset": ["data"],
                    "origin": "./samples.txt"
                }
            ],
            "base": ["Basic C Structure"]
        }
    ]
}
```

## Ground

`ground` is either `null` or the name of another composition that this composition builds upon.
When a ground is named, litar searches for `NAME/composer.json` (as a regular file) in the
following directories, in order:

1. the current working directory (`.`);
2. each directory in `$LITAR_LIBRARY` (a `$PATH`-style list);
3. `$HOME/.litar`;
4. `$HOME/.local/share/litar`;
5. `/usr/local/share/litar`;
6. `/usr/share/litar`.

The first match wins.
All nodes from the ground composition precede the nodes declared in this `composer.json`.

## Nodes

Each entry in `nodes` has:

- `name`: the node's name (must be non-empty, cannot contain `/`, and must be unique among
  siblings under the same parent);
- `archive`: path to the node's `.la` archive file, relative to `composer.json`
  (two nodes cannot share the same archive);
- `description`: a short description of the node;
- `mappings`: external file mappings for filter views;
- `base`: an array of node names giving the path from the root node down to this node's
  direct parent (`[]` for the root node).

The combined node list (ground nodes plus local nodes) must have a single root node.
A node's full path is its `base` names plus its own `name`, joined by `/`.
A node's block chain consists of all blocks along its `base` path (from the root down)
followed by the blocks in its own archive.
On the command line, `-f`/`--file` names an archive and evaluates at the node that owns
that archive.

## Mappings

The `mappings` array maps host files outside the archives into the read-only FUSE filesystem
seen by filters:

- `path`: destination path inside the filter's working directory;
- `fileset`: array of file-set names that include this file;
- `origin`: path to the host file, relative to `composer.json`.

A child node inherits all mappings from its `base` chain and may add more.

# Command-line name

Command-line arguments take qualified names, expressions, or items directly, without
surrounding `@<` and `@>` or a trailing end control; the name runs to the end of the argument.
Omitting `@/` on a chunk reference names the root module (there is no wildcard option that
matches a chunk across all modules).

- **`-p` / `--print` (`EXPRESSION`):** Takes a qualified name with an optional filter
  pipeline (and optional views), or a pipeline with an empty chunk name:
  ```
  label1@:label2@:module name@/chunk name@|filter1@|filter2
  ```
- **`-l` / `--literal`, `-e` / `--effective`, `-d` / `--depend` (`CHUNK_REFERENCE`):**
  Take a qualified name without a pipeline:
  - `-l` prints every matching block (including blocks applied to predecessors), with its
    preceding comment;
  - `-e` prints only the effective blocks of that chunk reference;
  - `-d` prints every block whose body content references, or whose pipeline uses, that
    chunk reference or a specialization of it.
- **`-r` / `--reference` (`ITEM`):** Takes an item path of one or more names separated by
  `@/` (without `@~` or a closing `@`), and prints every block carrying that exact item:
  ```
  domain@/subdomain@/item
  ```

The output format of `-e`, `-d`, and `-r` is identical to `-l`.
