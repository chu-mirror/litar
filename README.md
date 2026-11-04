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
  - src/cmd/litar-web: the `litar-web` command, a browser interface for compositions;
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

- `make` evaluates the Go files of `compos` into `tmp/build` and compiles `./litar` and `./litar-web`.
  With no `./litar` yet, it first compiles `tmp/boot/litar` from `src` to do the evaluation.
- `make sync` writes every file of `compos` into `src`.
- `make web` opens `compos` in `./litar-web` at <http://127.0.0.1:8080/> (`WEB_ADDR` changes it).
- `make install` installs `./litar` and `./litar-web` into `$(PREFIX)/bin`
  and the grounds under `$(PREFIX)/share/litar`; `PREFIX` defaults to `/usr/local`.
  It does not build, so run `make && sudo make install`.
- `make install-cursor` then sets up Cursor for every workspace, as the user and with the same `PREFIX`:
  it adds the `litar` MCP server, running the installed binary and grounds, to `~/.cursor/mcp.json`,
  keeping the other servers, and copies the skills of `agent/skills` into `~/.cursor/skills`.
  `install-mcp` and `install-skills` do each part; `CURSOR_DIR` changes `~/.cursor`.
- `make clean` removes `tmp/build`, `tmp/boot`, `./litar`, and `./litar-web`.

Edit `compos`, not `src`; `make sync` overwrites the files it lists.

# Browse

`litar-web` runs `litar -a` and serves a read-only page over its MCP resources:
the node tree, the chunks, files, and items of a node, its own blocks,
and for a chunk its specializations, effective, applied, and dependent blocks, and its result.
References, filters, items, and file headers in blocks are links.

```
LITAR_LIBRARY=grounds ./litar-web -litar ./litar -file examples/go/project-hello.la
```

It listens on `127.0.0.1:8080` by default (`-addr`).
The page may import compositions and evaluate expressions, which runs their filters,
but it cannot change an archive.

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
