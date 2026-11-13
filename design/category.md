The style guidance gives general advice about
how to create chunks and items and when to put a chunk under a module,
but that's not the whole story.
The chunks and items are categorized under modules and domains.
When the scale expands, a firm strategy to classify them becomes important.

The naming of items in [litar.md](./litar.md) is simplified, so it should not be used as a model.

# Chunks

A chunks tree might like the following:

- Tool: useful filters shared by all modules
- Language: common structures found in languages
  - C
    - Module: C source file and C header file
    - Project: the whole codebase, including build system
  - Go
    - File: Go source file
    - Package: Go package
    - Module: go.mod unit
  - (other languages)
- UI: the shared schema and generators
  - Command Line: the invocation syntax and the documents generated from it
    - C: generators of command line parsing in C
    - Go: generators of command line parsing in Go
  - Terminal
  - Graphical
- Framework: something an application chooses to adopt
  - (frameworks for building applications)
- Project
  - (concrete projects)
    - Build: the build system
    - Tests
    - Documents

Every chunk is free to refer to any other chunk.

# Items

An item path answers three questions in order:
in which context the name means something, what kind of thing it is, and what it is called.

```
<owner>@/<kind>@/<name>
```

## Owner

The owner is the narrowest context in which the name is unambiguous.
It is written with the same path as the module that owns the chunks of that context,
so chunks and items share one tree:

- `Tool`
- `Language@/Go`
- `UI@/Command Line`
- `Framework@/Light`
- `Project@/litar`

An item moves up to a wider owner only when a second owner starts using it.

## Kind

Kinds come from a short closed list.
A kind is a plural noun, like the name of a sequence in the style guidance.
There are two families of kinds.

Code kinds sit under the language,
because they are specific to the language and a project can mix languages:
`Go@/functions`, `Go@/methods`, `Go@/types`, `Go@/variables`, `C@/macros`, etc.

Knowledge kinds have no language level:

- `concepts`: one block defines it, other blocks depend on that definition;
- `terms`: a word given a narrower meaning in this context,
  the thing it names is described elsewhere or needs no definition;
- `topics`: a subject several blocks are about, with no single definition;
- `options`: a switch or setting that users face.

When a name could be a term or a concept, choose `concepts`.

## Name

A code item keeps the name as the language qualifies it, such as `litar.run`.
A knowledge item is named by a singular phrase,
not capitalized and without a period, the same as a chunk.

## Examples

```
Project@/litar@/Go@/functions@/litar.run
Project@/litar@/concepts@/effective block
Project@/litar@/terms@/view
Project@/litar@/topics@/filter cache
Project@/litar@/options@/--agent
Framework@/Light@/C@/macros@/LIGHT_TEST
Language@/Go@/concepts@/build constraint
```

## Rules

- **Write the definition in the reserved namespace `Index@/{item path}`**;
  the description of a concept or a term is its one-line definition,
  so the item tree works as a glossary.
- **Explain an item fully in the comment of the block that defines it**;
  every other block only carries the item.
- **Create an item only when it connects blocks**;
  at least two blocks carry it, or soon will.
- The cases to avoid cross-reference in the style guidance still apply.

