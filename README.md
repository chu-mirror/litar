# About the project

This project proposes a system named litar,
which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md](design/ui.md) defines a command line tool for handling litar archives,
including the MCP server it opens for agents.

# Directories

- design: design files describing what litar should be from a human's view;
- doc: references for the language and the implementation plan;
- compos: the composition of litar itself, the source of everything in `src`;
- src: the Go module of `litar`, synced from `compos`:
  - src/cmd/litar: the `litar` command;
  - src/internal/litar: the Go implementation;
  - src/tests: acceptance tests;
- grounds: library compositions that other compositions ground on;
- examples: example programs, composed by `examples/composer.json`;
- agent: skills and references for agents editing litar compositions,
  linked from `.cursor/skills`.

# Build and install

Prerequisites:

- Go 1.25 or later.
- FUSE (`/dev/fuse`) for the view tests.
  Those tests skip when it is absent.

The `Makefile` at the root builds litar from `compos`:

- `make` evaluates the Go files of `compos` into `tmp/build` and compiles `./litar`.
  With no `./litar` yet, it first compiles `tmp/boot/litar` from `src` to do the evaluation.
- `make sync` writes every file of `compos` into `src`.
- `make install` installs `./litar` as `$(PREFIX)/bin/litar`
  and the grounds under `$(PREFIX)/share/litar`; `PREFIX` defaults to `/usr/local`.
  It does not build, so run `make && sudo make install`.
- `make clean` removes `tmp/build`, `tmp/boot`, and `./litar`.

Edit `compos`, not `src`; `make sync` overwrites the files it lists.

# Test

The tests run inside `src`:

- `make -C src` builds `src/litar` with `go build`.
- `make -C src test` runs `go test ./...`, then the cases under `src/tests/`.

# Grounds

`grounds/` is a library of compositions.
Each subdirectory holds `composer.json`.
litar looks for a ground in the current directory, then in each directory of `$LITAR_LIBRARY`
(a `$PATH`-style list), then in `~/.litar`, `~/.local/share/litar`,
`/usr/local/share/litar`, and `/usr/share/litar`.

```
LITAR_LIBRARY=grounds ./litar -f examples/c/hello.la -p hello.c
```
