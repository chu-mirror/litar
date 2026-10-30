# About the project

This project proposes a system named litar,
which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md](design/ui.md) defines a command line tool for handling litar archives,
including the MCP server it opens for agents.

# Directories

- design: design files describing what litar should be from a human's view;
- doc: references for the language and the implementation plan;
- cmd: the `litar` command;
- internal/litar: the Go implementation;
- lib: litar libraries, composed by `lib/composer.json`;
- examples: example programs, composed by `examples/composer.json`;
- tests: acceptance tests.

# Build and test

Prerequisites:

- Go 1.25 or later.
- FUSE (`/dev/fuse`) for the view tests.
  Those tests skip when it is absent.

Procedures:

- `make` builds `./litar` with `go build -o litar ./cmd/litar`.
- `make test` runs `go test ./...`, then the cases under `tests/`.
- `make clean` removes the binary.
