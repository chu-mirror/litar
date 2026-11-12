# About the project

This project proposes a system named litar,
which is defined in [litar.md](design/litar.md).
[style.md](design/style.md) explains the construction of a litar archive.
[ui.md](design/ui.md) defines a command line tool for handling litar archives,
including the MCP server it opens for agents.
[category.md](design/category.md) gives the strategy to classify chunks and items.

# Directories

- design: design files describing what litar should be from a human's view;
- doc: references for the language and the implementation plan;
- compos: the composition of litar itself, the source of everything in `src`;
- src: the Go module of `litar`, synced from `compos`:
  - src/cmd/litar: the `litar` command;
  - src/cmd/litar-web: the `litar-web` command, a browser interface for compositions;
  - src/internal/litar: the Go implementation;
  - src/tests: acceptance tests;
- standard: the standard library, a composition that other compositions ground on;
- examples: example programs, composed by `examples/composer.json`;
- agent: skills and references for agents editing litar compositions,
  linked from `.cursor/skills`.

# Build and install

Prerequisites:

- Go 1.25 or later.
- Python 3.10 or later, for the text processing filters of the grounds,
  which read their configuration as Jsonnet and validate it with pydantic.
  When `python3` cannot import `pydantic` and `jsonnet`, the filters run through
  [uv](https://docs.astral.sh/uv/), which downloads them on the first run.
  The filters run in a background Python process that loads them once.
- FUSE (`/dev/fuse`) for the view tests.
  Those tests skip when it is absent.

The `Makefile` at the root builds litar from `compos`.
The build itself is the composition in `build`, whose ground is `compos`:
its node maps `bin` and `src` of the repository into the filter views,
and each target evaluates one of its chunks.

- `make` compiles `bin/litar` and `bin/litar-web` from the Go files of `compos`.
  With no `bin/litar` yet, it first compiles `tmp/boot/litar` from `src` to do the evaluation.
- `make sync` writes the files of `compos`, except the Go test files, into `src`.
- `make test` runs `go test` on `compos` in a filter view.
- `make web` opens `compos` in `bin/litar-web` at <http://127.0.0.1:8080/> (`WEB_ADDR` changes it).
- `make install` installs `bin/litar` and `bin/litar-web` into `$(PREFIX)/bin`
  and the grounds under `$(PREFIX)/share/litar`; `PREFIX` defaults to `/usr/local`.
  It does not build, so run `make && sudo make install`.
- `make install-cursor` then sets up Cursor for every workspace, as the user and with the same `PREFIX`:
  it adds the `litar` MCP server, running the installed binary and grounds, to `~/.cursor/mcp.json`,
  keeping the other servers, and copies the skills of `agent/skills` into `~/.cursor/skills`.
  `install-mcp` and `install-skills` do each part; `CURSOR_DIR` changes `~/.cursor`.
- `make clean` removes `bin` and `tmp/boot`.

Edit `compos`, not `src`; `make sync` overwrites the files it lists.

# Filter cache

litar keeps the output of each filter run in `~/.cache/litar`,
under its script, its input, and the files of its view,
and a later run with all of them unchanged reuses that output instead of starting the filter.
A run is not kept when its view has a mapped file or directory, or when it writes to stderr.
`LITAR_CACHE` names another directory; `LITAR_CACHE=off` turns the cache off.
Filters see the directory in `LITAR_CACHE`; remove the whole directory to clear the cache.
A filter that reads anything else, such as the network or the clock, must not rely on running every time.

# Browse

`litar-web` runs `litar -a` and serves a read-only page over its MCP resources:
the node tree, the chunks, files, and items of a node, its own blocks,
and for a chunk its specializations, effective, applied, and dependent blocks, and its result.
References, filters, items, and file headers in blocks are links.

```
bin/litar-web -litar bin/litar -file examples/go/project-hello.la
```

It listens on `127.0.0.1:8080` by default (`-addr`).
The page may import compositions and evaluate expressions, which runs their filters,
but it cannot change an archive.

# Test

- `make test` runs `go test ./...` in a filter view of `compos`.
  The Go test files live only in `compos`; `make sync` does not write them into `src`.
  The view maps `examples` and `standard` into `internal/litar/testdata` for the tests that read them.
- `make -C src` builds `src/litar` with `go build`.
- `make -C src test` runs the cases under `src/tests/` with that binary.

# Grounds

`standard/` is the standard library of litar, the ground of the other compositions here.
It is a single node whose modules follow
[category.md](design/category.md): shared filters under `Tool`, the structures of C and Go under `Language`,
the command line generators under `UI@/Command Line`, and the framework Light under `Framework`.
litar looks for a ground in the current directory, then in each directory of `$LITAR_LIBRARY`
(a `$PATH`-style list), then in `~/.litar`, `~/.local/share/litar`,
`/usr/local/share/litar`, and `/usr/share/litar`.

```
bin/litar -f examples/c/hello.la -p hello.c
```
