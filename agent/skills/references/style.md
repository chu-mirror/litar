# litar authoring style

A digest of the design files `style.md` and `category.md` of the litar project.

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

## Where a chunk goes

Modules form one tree; every chunk may refer to any other.

| Module | Holds |
| --- | --- |
| `Tool` | filters shared by all modules |
| `Language@/<language>@/<level>` | structures of a language: `C@/Module`, `Go@/File`, `Go@/Module`, … |
| `UI@/Command Line`, `UI@/Terminal`, `UI@/Graphical` | shared schemas and generators of an interface |
| `Framework@/<framework>` | something an application chooses to adopt |
| `Project@/<project>` | a concrete project; its `Build`, `Tests`, `Documents` below it |

- The root module holds only file chunks.
- A chunk that other modules need not see goes into a submodule named after its subject
  (`Format`, `Meta`). This only keeps it out of the way; it may still be referred to.
- Refer to a submodule chunk relatively, `@<.@/Output@/print hello to stdout@>`;
  inside the submodule, reach the module's own chunks with `..@/`.
- Nodes are for blocks that contradict each other, not for grouping. A task may use a node
  while the placement of its blocks is undecided.

## Blocks

- Keep the content of a block at or below about 150 tokens (comments and items do not count);
  divide a bigger tree into chunks.
- Put a comment before a block that introduces a decision
  (a buffer size, failure handling, an algorithm choice).
- Make blocks extending a sequence order-irrelevant. When order matters
  (an if-else chain), gather those parts into one block, even if it gets long.
- Generate several forms of one piece of information (help text, parser, manpage)
  from one declarative chunk through filters instead of keeping copies.
- Put the build of a project in a node based on the project's, `Project@/<project>@/Build`:
  mappings to the real directories, build scripts, and what leaves the archive.

## Dividing a long block

- Make it an outline: each step becomes a reference on a line of its own,
  named after its action, with the objects it works on quoted and declared above it.
- Split a switch with distinct constant cases into a sequence
  (`cases of 'opt' in 'run'`): start with `@=`, add the rest with `@+`.
- Leave whole: generated or data blocks, and order-dependent structures.
- Expansion indents the content of an indented reference. Write a sub-chunk unindented
  and indent its reference to the level of the code around it.
- Content with a string or here-document over several lines must not be indented:
  start the line with its reference.
- End a tree sub-chunk without an empty line before `@`, or a blank line follows the reference.
  Sequence blocks without a trailing empty line are joined by one line feed.

## Items

An item is `<owner>@/<kind>@/<name>`.

- **Owner:** the narrowest context in which the name is unambiguous, written as the module path
  that owns it: `Tool`, `Language@/Go`, `UI@/Command Line`, `Framework@/Light`, `Project@/litar`.
  Move an item to a wider owner only when a second owner starts using it.
- **Kind:** a plural noun from a short closed list.
  Code kinds sit under the language: `Go@/functions`, `Go@/methods`, `Go@/types`, `C@/macros`.
  Knowledge kinds have no language:
  `concepts` (one block defines it), `terms` (a word given a narrower meaning),
  `topics` (a subject of several blocks), `options` (a switch users face).
  Between a term and a concept, choose `concepts`. Never add a synonym of an existing kind.
- **Name:** a code item as the language qualifies it (`litar.run`);
  a knowledge item as a singular phrase, like a chunk name.

```
Project@/litar@/Go@/functions@/litar.run
Project@/litar@/concepts@/effective block
Project@/litar@/options@/--agent
Framework@/Light@/C@/macros@/LIGHT_TEST
```

- Mark an item with `@~` when its context reaches beyond the current block, on the block
  that defines it and on every block that uses it.
- Do not mark items local to the block, common-sense items (standard library,
  well-known libraries), or items quoted in the chunk name.
- Create an item only when it connects blocks: at least two carry it, or soon will.
- Write its definition in `Index@/{item path}`; explain it fully in the comment of the
  block that defines it.
- A well-defined unit such as a function goes directly into its sequence with an item,
  not into a chunk of its own:

```
@<Language@/C@/Module@/functions@+
void hello()
{
    printf("Hello\n");
}
@~ Project@/hello@/C@/functions@/hello @
```
