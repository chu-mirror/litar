# About the project

This project proposes a system named litar, which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md](design/ui.md) defines a command line tool for handling litar archives.
[agent.md](design/agent.md) defines how an agent inspects and edits archives through MCP.

# Directories

- design: design files describing what litar should be from a human's view;
- doc: useful references for understanding litar;
- src: the literate source, the tangled C translation unit, and the MCP server;
- lib: litar libraries;
- examples: example litar archives;
- tests: tests for the features implemented so far.

# Build and test

Prerequisites:

- C11 compatitable C compiler;
- uv.

Procedures:

- `src/litar.la` is the literate source of the program. It includes `lib/c-structure.la`.
- `make tangle` regenerates `src/litar.c` from that archive. Tangling needs a `./litar` binary, so the current `src/litar.c` is compiled first.
- `make` compiles `src/litar.c` into `./litar`.
- `make test` compiles it, then runs the tests under `tests/`.
- `make clean` removes the binary.

