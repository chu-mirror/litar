# litar authoring style

## Structure

A chunk encapsulates a structure of one of two shapes:
a **sequence** (a series of same-kind parts: includes, functions) or a
**tree** (a fixed composition: a file, a function body).

## Naming chunks

- A chunk name is a short phrase describing the structure; no leading capital, no final period.
- A sequence: plural noun phrase (`includes`, `global variables`).
- An action: verb phrase with concrete verbs (not "perform", "do"), quoting the objects
  operated on: `save the sum of 'number1' and 'number2' to 'number3'`.
  Declare every quoted object before the reference that quotes it.
  Omit the quotes when many chunks refer to the same objects.
- Do not use an object declared inside a chunk outside that chunk; declare it in the
  block whose expansion holds all its uses.
- Anything else: noun phrase (`main body`, `invocation syntax`).

## Modules

- Hide what other modules need not see: a chunk that shall not be referred to outside its
  module goes into a submodule named after its subject, such as `Output` or `Meta`.
  Other modules may still refer to it; the submodule only keeps it out of the way.
- Keep in the module itself the chunks other modules use: the parts of a template
  (`includes`, `functions`), hooks they extend, and outputs they quote.
- Refer to a submodule chunk relatively, `@<.@/Output@/print hello to stdout@>`;
  inside the submodule, reach the module's own chunks with `..@/`.
- Put the non-file chunks of a project in a module named after it (`Litar`),
  not in the root module, which holds the file chunks.

```
@<C Structure@/definitions@+
void print_hello()
{
    @<.@/Output@/print hello to stdout@>
}
@

@<C Structure@/Output@/print hello to stdout@=
printf("Hello\n");
@
```

## Blocks

- Keep a block at or below about 150 tokens; divide a bigger tree into chunks.
- Put a comment before a block that introduces a decision
  (a buffer size, failure handling, an algorithm choice).
- Make blocks extending a sequence order-irrelevant. When order matters
  (e.g. an if-else chain), gather those parts into one block, even if it gets long.
- Generate several forms of one piece of information (help text, parser, manpage)
  from one declarative chunk through filters instead of keeping copies.

## Cross-references

- Mark an item with `@~` when its context reaches beyond the current block.
- Do not mark items local to the block, common-sense items (standard library,
  well-known libraries), or items quoted in the chunk name.
- A well-defined unit such as a function goes directly into its sequence with an item,
  not into a chunk of its own:

```
@<functions@+
void hello()
{
    printf("Hello\n");
}
@~ C@/functions@/hello @
```