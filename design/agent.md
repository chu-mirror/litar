Agent interact with litar through MCP.

The implementation of the MCP server provides tools to
inspect and modify a litar archive;
the tools are based on blocks and chunks:

1. fetch all modules and their description,
2. check blocks applied to a chunk reference,
3. check blocks depending on a chunk reference,
4. evaluate the content of an expression,
5. modify a certain block,
6. delete a certain block,
7. append a block to the archive.

The MCP server can not read or edit a litar archive directly,
all the operations are based on two command line tools, litar and sed.

The `-a`/`--agent` option of litar will let it provide all necessary information
to locate a block, and modify the format of output.

Here's a sample archive, named `archive.la`, from [ui.md](ui.md):

```
@- module 1 @

@<chunk@=
@<substructure 1@>
@

@<chunk2@=
@<substructure@>
@

@- module 2 @

Extend chunk through label.
@<label@:module 1@/chunk@+
@<substructure 2@>
@
```

If we run `litar -a -l label@:module 1@/chunk` for this, it should print,

```
[
    {
        "comment": "",
        "labels": [],
        "module": "module 1",
        "chunk": "chunk",
        "type": "set",
        "content": "@<substructure 1@>\n",
        "filters": []
        "file": "archive.la",
        "line-range": [2, 5],
    },
    {
        "comment": "Extend chunk through label.\n",
        "labels": ["label"],
        "module": "module 1",
        "chunk": "chunk",
        "type": "extend",
        "content": "@<substructure 2@>\n",
        "filters": []
        "file": "archive.la",
        "line-range": [13, 16],
    }
]
```

