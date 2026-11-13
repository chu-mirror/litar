# Build litar from the composition in litar, sync src from it, and install.
#   make                  compile litar and litar-web into $(BIN)
#   make sync             write the files of the composition, except the Go tests, into $(SRC)
#   make test             build, then run the Go tests and the acceptance cases in invocations
#   make boot             compile $(BOOT) from the Go sources in $(SRC)
#   make web              browse $(ARCHIVE) with $(BIN)/litar-web on $(WEB_ADDR)
#   make install          install $(BIN)/litar, $(BIN)/litar-web, and the grounds under $(PREFIX)
#   make install-bin      install $(BIN)/litar and $(BIN)/litar-web into $(BINDIR)
#   make install-grounds  copy each ground composition into $(GROUNDDIR)
#   make install-cursor   install the MCP server and the agent skills for Cursor under $(CURSOR_DIR)
#   make install-mcp      add the litar server, run as $(BINDIR)/litar, to $(MCP_CONFIG)
#   make install-skills   copy each directory of $(SKILLS) into $(CURSOR_DIR)/skills
#   make clean            remove $(BIN) and $(BOOT)
#
# The build itself is the node Litar Build of that composition, based on the node Litar;
# its mappings put $(BIN) and $(SRC) into the filter views, so each target here evaluates one chunk
# in one litar process, and every file is evaluated once.
# LITAR is the binary that evaluates it: $(BIN)/litar from an earlier build,
# else $(BOOT), which is compiled from $(SRC) first. So a fresh checkout needs only go,
# and `make sync` keeps $(SRC) able to boot the next build.
# The Go test files are not synced: `make test` runs them in a filter view of the composition,
# where the node Litar maps examples and the ground standard as their test data,
# and then the acceptance cases on the commands in $(BIN).
# Evaluation reads the ground standard from this repository, not the installed one.
# sync overwrites the files the composition lists and leaves any other file.
#
# PREFIX defaults to /usr/local; DESTDIR is prepended to every installed path.
# litar finds grounds in /usr/local/share/litar, /usr/share/litar, and ~/.local/share/litar;
# for any other PREFIX, add $(GROUNDDIR) to LITAR_LIBRARY.
# install does not build, so `make && sudo make install` compiles as the user.
#
# install-cursor writes to the home directory, so run it as the user, after install, with the same PREFIX.
# The server entry names the installed binary and grounds, so it works in any workspace.
# install-mcp replaces only the litar entry of $(MCP_CONFIG) and keeps the other servers.
# The skills link to each other and to references by relative paths, so they are copied side by side.

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
GROUNDDIR ?= $(PREFIX)/share/litar

ARCHIVE := litar/litar.la
BUILD_ARCHIVE := litar/build.la
# The ground compositions of the repository, each a directory at its root.
GROUND_NAMES := standard
# The mappings in litar/composer.json name these directories.
BIN := bin
SRC := src
BOOT ?= tmp/boot/litar
WEB_ADDR ?= 127.0.0.1:8080
COMMANDS := $(addprefix $(BIN)/,litar litar-web)
CURSOR_DIR ?= $(HOME)/.cursor
MCP_CONFIG ?= $(CURSOR_DIR)/mcp.json
SKILLS := agent/skills

LITAR ?= $(or $(wildcard $(CURDIR)/$(BIN)/litar),$(abspath $(BOOT)))

SKILL_NAMES := $(notdir $(wildcard $(SKILLS)/*))
BOOT_SOURCES := $(SRC)/go.mod $(SRC)/go.sum $(shell find $(SRC)/cmd $(SRC)/internal -name '*.go' 2>/dev/null)
EVAL = LITAR_LIBRARY=$(CURDIR) $(LITAR) -f $(abspath $(BUILD_ARCHIVE))
# Build the bootstrap binary only when it is the one evaluating.
EVALUATOR := $(filter $(abspath $(BOOT)),$(LITAR))

.PHONY: all litar sync test boot web install install-bin install-grounds
.PHONY: install-cursor install-mcp install-skills mcp clean

all: litar

boot: $(abspath $(BOOT))

$(abspath $(BOOT)): $(BOOT_SOURCES)
	mkdir -p $(dir $@)
	cd $(SRC) && GOWORK=off go build -o $@ ./cmd/litar

litar: $(EVALUATOR)
	$(EVAL) -p 'Project@/litar@/Build@/commands'

sync: $(EVALUATOR)
	$(EVAL) -p 'Project@/litar@/Build@/source files'

test: litar
	$(EVAL) -p 'Project@/litar@/Tests@/go test report'
	$(EVAL) -p 'Project@/litar@/Tests@/acceptance report'

web:
	@test -x $(BIN)/litar-web || { echo "$(BIN)/litar-web is missing; run make first" >&2; exit 1; }
	LITAR_LIBRARY=$(CURDIR) $(BIN)/litar-web -addr $(WEB_ADDR) -litar $(abspath $(BIN))/litar -file $(ARCHIVE)

install: install-bin install-grounds

install-bin:
	@for c in $(COMMANDS); do test -x $$c || { echo "$$c is missing; run make first" >&2; exit 1; }; done
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 $(COMMANDS) "$(DESTDIR)$(BINDIR)/"

install-grounds:
	for g in $(GROUND_NAMES); do \
		rm -rf "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		install -d "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		cp -R "$$g/." "$(DESTDIR)$(GROUNDDIR)/$$g/" || exit 1; \
	done

install-cursor: install-mcp install-skills

install-mcp: $(EVALUATOR)
	@test -x "$(abspath $(BINDIR))/litar" || { echo "$(abspath $(BINDIR))/litar is missing; run make install first" >&2; exit 1; }
	script=$$($(EVAL) -p 'Project@/litar@/Build@/MCP configuration script') && \
	python3 -c "$$script" "$(MCP_CONFIG)" "$(abspath $(BINDIR))/litar" "$(abspath $(GROUNDDIR))"

install-skills:
	for s in $(SKILL_NAMES); do \
		rm -rf "$(CURSOR_DIR)/skills/$$s" && \
		install -d "$(CURSOR_DIR)/skills/$$s" && \
		cp -R "$(SKILLS)/$$s/." "$(CURSOR_DIR)/skills/$$s/" || exit 1; \
	done
	find "$(CURSOR_DIR)/skills" -name __pycache__ -prune -exec rm -rf {} +

mcp:
	npx @modelcontextprotocol/inspector

clean:
	rm -rf $(BIN) $(dir $(BOOT))
