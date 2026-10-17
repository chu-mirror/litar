# About the project

This project proposes a system named litar, which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md] defines a command line tool for handling litar archives.

# Directories

- design: design files describing what this project should be from human's view;
- plugin: plugin for an agent to interact with litar;
- examples: some examples written in litar format;
- src: the generated source files;
- tests: tests for the features implemented so far.

# Build and test

- `src/litar.la` is the literate source of the program. It includes `lib/c-structure.la`.
- `make tangle` regenerates `src/litar.c` from that archive. Tangling needs a `./litar` binary, so the current `src/litar.c` is compiled first.
- `make` compiles `src/litar.c` into `./litar`.
- `make test` compiles it, then runs the tests under `tests/`.
- `make clean` removes the binary.
- `make grok` set up grok build.
