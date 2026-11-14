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
- litar: the composition of litar itself: the node `Litar` is the source of everything in `src`,
  and the node `Litar Build`, based on it, builds and tests litar;
- src: the Go module of `litar`, synced from the composition `litar`:
  - src/cmd/litar: the `litar` command;
  - src/cmd/litar-web: the `litar-web` command, a browser interface for compositions;
  - src/internal/litar: the Go implementation;
- standard: the standard library, a composition that other compositions ground on;
- examples: example programs, composed by `examples/composer.json`;
- agent: skills for agents working on litar compositions (`litar-task` for editing,
  `litar-port` for porting a project, `litar-explain` for explaining), with their references,
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

The `Makefile` at the root builds litar from the composition `litar`.
The build itself is the node `Litar Build` of that composition, based on the node `Litar`:
it maps `bin` and `src` of the repository into the filter views,
and each target evaluates one of its chunks.

- `make` compiles `bin/litar` and `bin/litar-web` from the Go files of the composition.
  With no `bin/litar` yet, it first compiles `tmp/boot/litar` from `src` to do the evaluation.
- `make sync` writes the files of the node `Litar`, except the Go test files, into `src`,
  with `go.mod` and `go.sum` of the composition.
- `make mod` runs `go mod edit` and `go mod tidy` on `litar/mod/go.mod` and `litar/mod/go.sum`,
  from the requirements chunk of the composition. The go command manages the two files,
  so they are not in the archives; the node `Litar` maps their directory into the filter views.
- `make test` runs the Go tests and the acceptance cases, each in a filter view.
- `make web` opens the node `Litar` in `bin/litar-web` at <http://127.0.0.1:8080/> (`WEB_ADDR` changes it).
- `make install` installs `bin/litar` and `bin/litar-web` into `$(PREFIX)/bin`
  and the grounds under `$(PREFIX)/share/litar`; `PREFIX` defaults to `/usr/local`.
  It does not build, so run `make && sudo make install`.
- `make install-cursor` then sets up Cursor for every workspace, as the user and with the same `PREFIX`:
  it adds the `litar` MCP server, running the installed binary and grounds, to `~/.cursor/mcp.json`,
  keeping the other servers, and copies the skills of `agent/skills` into `~/.cursor/skills`.
  `install-mcp` and `install-skills` do each part; `CURSOR_DIR` changes `~/.cursor`.
- `make clean` removes `bin` and `tmp/boot`.

Edit the composition `litar`, not `src`; `make sync` overwrites the files it lists.

# Filter cache

litar keeps the output of each filter run in `~/.cache/litar`,
under its script, its input, and the files of its view,
and a later run with all of them unchanged reuses that output instead of starting the filter.
A run is not kept when its view has a mapped file or directory, or when it writes to stderr.
`LITAR_CACHE` names another directory; `LITAR_CACHE=off` turns the cache off.
Filters see the directory in `LITAR_CACHE`; remove the whole directory to clear the cache.
A filter that reads anything else, such as the network or the clock, must not rely on running every time.

# Browse and edit

`litar-web` runs `litar -a` and serves a page over its MCP resources and tools,
so the page can do what an agent can:

- browse the nodes, and the chunks, files, items, and label paths of a node;
- read a chunk under a label path: its effective and applied blocks, the blocks that use it, and its result.
  A reference in a block is a link, and opens the blocks of its chunk in place;
  a reference that no block defines is marked. Items are chips that lead to the blocks carrying them;
- edit: add, change, move, and delete blocks; rename and delete chunks, modules, items, and files;
  describe chunks, files, items, nodes, and the composition; create and delete nodes;
- search the blocks of a node, evaluate an expression, and call any tool of the server
  from a form built from its input schema (Tools).

```
bin/litar-web -litar bin/litar -file examples/go/project-hello.la
```

It listens on `127.0.0.1:8080` by default (`-addr`).
The page writes archives and runs filters, so the server answers only requests whose host is
`localhost` or an IP address and whose origin is the page itself.
It has its own `litar` process: what an agent or an editor writes to the archives is loaded
when the window gets the focus again, or with Reload.

# Test

- `make test` runs `go test ./...` in a filter view of the composition.
  The Go test files live only in the composition; `make sync` does not write them into `src`.
  The view maps `examples` and `standard` into `internal/litar/testdata` for the tests that read them.
- `make test` then runs the acceptance cases on `bin/litar`, which it builds first.
  A case is a chunk of the node `Litar Build` under `Project@/litar@/Tests@/Cases`: its content is a script of checks,
  and the filter `UI@/Command Line@/Test@/run case` of the ground `standard` turns it into its result,
  so evaluating the chunk runs the case.
  `Project@/litar@/Tests@/acceptance report` lists every result and fails when a case failed.
  One case runs alone with
  `LITAR_LIBRARY=. bin/litar -f litar/build.la -p 'Project@/litar@/Tests@/Cases@/print'`.

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
