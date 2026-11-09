# Synopsis

litar is a command line tool that supports both short options and long options.

```
usage: litar [options]

--help: print the usage
-a, --agent: open an MCP server to handle the requests from agents
-f, --file ARCHIVE: specify the archive file, find the composition and the node corresponding
-l, --literal CHUNK_REFERENCE: print the blocks applied to CHUNK_REFERENCE literally
-e, --effective CHUNK_REFERENCE: print the effective blocks of CHUNK_REFERENCE
-d, --depend CHUNK_REFERENCE: list all blocks that depend on CHUNK_REFERENCE
-r, --reference ITEM: list all blocks that refer to ITEM
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
```

The formats:

- NODE: the names defined in the composer file;
- CHUNK_REFERENCE: `label1@:label2@:module name@/chunk name`;
- EXPRESSION: `label1@:label2@:module name@/chunk name@|filter1@|filter2`;
- ITEM: `domain@/subdomain@/item`.

# Explanation

Printing the blocks literally means, for each block, printing the comment before the block
and the block expression just as it is in the file.

When the printed chunk reference is specialized,
all the blocks applied to the predecessors of that chunk reference will be printed too.
For example,

```
@<module@/chunk1@=
@<substructure 1@>
@

@<module@/chunk2@=
@<substructure 2@>
@

Extend chunk through label.
@<label@:module@/chunk1@=
@<substructure 3@>
@
```

If we run `litar -l 'label@:module@/chunk1'` for this, it should print,

```
@<module@/chunk1@=
@<substructure 1@>
@

Extend chunk through label.
@<label@:module@/chunk1@=
@<substructure 3@>
@
```

A block depends on a chunk reference when its content contains, or is filtered by,
the chunk reference or a chunk reference specialized from the chunk reference.

```
@<chunk 1@=
content
@

@<chunk 2@=
@<label 1@:chunk 1@>
@

@<chunk 3@=
content
@|label 2@:chunk 2@
```

The second block depends on `chunk 1` and `label 1@:chunk 1`;
the third block depends on `chunk 2` and `label 2@:chunk 2`.

The output format of option `-e`, `-d`, and `-r` agrees with `-l`.

# MCP Server

The MCP server supports protocol version
[2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28).

The implementation of the MCP server opened through option `-a` provides context
to deal with compositions and tools to inspect and modify nodes.

Resources:

- compositions, `list://composition`, a list of editable compositions;

Resource templates:

- nodes, `tree://node/{composition}`, the node tree under the composition.
- chunks, `tree://chunk/{composition}/{node}`, the chunk tree organized by modules under the node,
  the file chunks are treated differently, they are displayed under `File@/<file path separated by '@/'>`,
  the same as reserved internal chunks, rather than literal file paths;
- items, `tree://item/{composition}/{node}`, the item tree under the node;
- files, `tree://file/{composition}/{node}`, the file tree under the node;
- node blocks, `blocks://node/{composition}/{node}`, the blocks in the node;
- specializations, `tree://specialization/{composition}/{node}/{chunk}`, the specializations on the chunk;
- applied blocks, `blocks://apply/{composition}/{node}/{chunk}/{specialization}`,
  the blocks applied to the chunk reference;
- effective blocks, `blocks://effective/{composition}/{node}/{chunk}/{specialization}`,
  the effective blocks of the chunk reference;
- dependent blocks, `blocks://depend/{composition}/{node}/{chunk}/{specialization}`,
  the blocks depending on a chunk reference;
- related blocks, `blocks://refer/{composition}/{node}/{item}`, the blocks referring to the item;
- file, `file://{composition}/{node}/{path}`, the file of the path.

Tools:

- import an existing composition;
- create a new composition, and import it;
- create a new node, the corresponding archive file must be newly created;
- delete a node, all nodes based on that node will be deleted too, all related archive files will be deleted;
- delete a chunk, all applied blocks will be deleted;
- delete a file, all applied blocks will be deleted;
- evaluate the content of an expression;
- apply a block to a node, cannot apply a block to a node in the ground archive.

`list://` returns a list of json objects:

```
[
    {
        "name": "<the name of the object>",
        "description": "<a short description of the object>",
        "id": <automatically generated unique id>
    },
    <other objects>
]
```

The name for a composition is its absolute path.

`tree://` returns a tree-like json object:

```
{
    "name": "<the name of the object>",
    "description": "<a short description of the object>",
    "id": <automatically generated unique id>,
    "sub-trees": [
        <sub-trees>
    ]
}
```

The generated id is a number counting from 0,
and must be consistent among requests if the returned list and tree do not change.
The arguments passed to tools and resource templates are these ids.

`blocks://` returns a list of blocks just as the output of option `-l`.

A particular block is removed by overriding through `@=`.
For example, an `@+` block is deleted by creating a single overriding `@=` and a new list of `@+`.
Remove a block when it is no longer an effective block of the same chunk reference in that same node.

An example for deleting:

```
@<includes@+
#include <stdio.h>
@

@<includes@+
#include <stdlib.h>
@

@<includes@+
#include <unistd.h>
@
```

Now we want to remove `#include <stdlib.h>`, the proper way:

```
@<includes@=
#include <stdio.h>
@

@<includes@+
#include <unistd.h>
@
```

# Environment

litar searches the ground compositions in this order:

1. current directory;
2. directories in $LITAR_LIBRARY, in the same format as $PATH;
3. $HOME/.litar;
4. $HOME/.local/share/litar;
5. /usr/local/share/litar;
6. /usr/share/litar.

