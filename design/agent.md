An agent interact with litar through MCP.

The implementation of the MCP server provides tools to
inspect and modify a litar archive;
the tools are based on blocks and chunks:

- fetch modules under a module and get their description,
- fetch chunks under a module,
- check blocks applied to a chunk reference,
- check effective blocks of a chunk reference,
- check blocks depending on a chunk reference,
- check blocks that refer to a topic,
- evaluate the content of an expression,
- modify a certain block,
- delete a certain block,
- append a block to the archive.

The MCP server can not read or edit a litar archive directly,
all the operations are based on two command line tools, litar and sed.

The `-a`/`--agent` option of litar will let it provide all necessary information
to locate a block, and modify the format of output.

Here's a sample archive, named `archive.la`, from [ui.md](ui.md):

```
@<module1@/chunk1@=
@<.@/substructure 1@>
@

@<module1@/chunk2@=
@<.@/substructure@>
@

Extend chunk through label.
@<label@:module1@/chunk1@+
@<.@/substructure 2@>
@~ topic @
```

If we run `litar -a -l label@:module 1@/chunk` for this, it should print,

```
[
    {
        "comment": "",
        "labels": [],
        "module": "module1",
        "chunk": "chunk",
        "type": "set",
        "content": "@<.@/substructure 1@>\n",
        "filters": [],
        "topics": [],
        "file": "archive.la",
        "line-range": [1, 3]
    },
    {
        "comment": "Extend chunk through label.\n",
        "labels": ["label"],
        "module": "module1",
        "chunk": "chunk",
        "type": "extend",
        "content": "@<.@/substructure 2@>\n",
        "filters": [],
        "topics": ["topic"],
        "file": "archive.la",
        "line-range": [9, 12]
    }
]
```

