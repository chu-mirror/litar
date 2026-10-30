# About the project

This project proposes a system named litar, which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md](design/ui.md) defines a command line tool for handling litar archives,
including the MCP server it opens for agents.

# Directories

- design: design files describing what litar should be from a human's view;
- doc: useful references for understanding litar;
- src: the C source of the program;
- lib: litar libraries;
- examples: example litar archives;
- tests: tests for the features implemented so far.

# Build and test

Prerequisites:

- C11 compatible C compiler.

Procedures:

- `make` compiles `src/litar.c` into `./litar`.
- `make test` compiles it, then runs the tests under `tests/`.
- `make clean` removes the binary.

