# Build litar from the composition in litar, sync src from it, and install.
#   make                  compile litar and litar-web into $(BIN)
#   make sync             write the files of the composition, except the Go tests, into $(SRC)
#   make mod              bring go.mod and go.sum of the composition in step with its requirements
#   make test             build, then run the Go tests and the acceptance cases in invocations
#   make boot             compile $(BOOT) from the Go sources in $(SRC)
#   make web              browse $(ARCHIVE) with $(BIN)/litar-web on $(WEB_ADDR)
#   make install          install $(BIN)/litar, $(BIN)/litar-web, and the grounds under $(PREFIX)
#   make install-bin      install $(BIN)/litar and $(BIN)/litar-web into $(BINDIR)
#   make install-grounds  copy each ground composition into $(GROUNDDIR)
#   make install-cursor       install the litar plugin for Cursor under $(CURSOR_DIR)/plugins/local/litar
#   make install-claude       install the litar plugin for Claude Code via $(CLAUDE_DIR)/local-marketplaces/litar
#   make install-antigravity  install the litar plugin for Antigravity under $(ANTIGRAVITY_DIR)/plugins/litar
#   make clean                remove $(BIN) and $(BOOT)
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
# go.mod and go.sum are not in the archives: the go command manages them in the directory mod
# of the composition, which the node Litar maps into the views. `make mod` runs go mod edit and
# go mod tidy on them from the requirements chunk; `make sync` copies them into $(SRC).
#
# PREFIX defaults to /usr/local; DESTDIR is prepended to every installed path.
# litar finds grounds in /usr/local/share/litar, /usr/share/litar, and ~/.local/share/litar;
# for any other PREFIX, add $(GROUNDDIR) to LITAR_LIBRARY.
# install does not build, so `make && sudo make install` compiles as the user.
#
# install-cursor, install-claude, and install-antigravity install the plugin in $(PLUGIN),
# dereferencing its internal symlinks. Run them as the user after `make install` (with `litar` on PATH).
# Claude Code requires a marketplace to persist a local plugin, so install-claude stages a local one
# under $(CLAUDE_DIR)/local-marketplaces/litar and registers it with `claude plugin`.

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
CLAUDE_DIR ?= $(HOME)/.claude
ANTIGRAVITY_DIR ?= $(HOME)/.gemini/config
PLUGIN := agent

LITAR ?= $(or $(wildcard $(CURDIR)/$(BIN)/litar),$(abspath $(BOOT)))

BOOT_SOURCES := $(SRC)/go.mod $(SRC)/go.sum $(shell find $(SRC)/cmd $(SRC)/internal -name '*.go' 2>/dev/null)
EVAL = LITAR_LIBRARY=$(CURDIR) $(LITAR) -f $(abspath $(BUILD_ARCHIVE))
# Build the bootstrap binary only when it is the one evaluating.
EVALUATOR := $(filter $(abspath $(BOOT)),$(LITAR))

.PHONY: all litar sync mod test boot web install install-bin install-grounds
.PHONY: install-cursor install-claude install-antigravity mcp clean

all: litar

boot: $(abspath $(BOOT))

$(abspath $(BOOT)): $(BOOT_SOURCES)
	mkdir -p $(dir $@)
	cd $(SRC) && GOWORK=off go build -o $@ ./cmd/litar

litar: $(EVALUATOR)
	$(EVAL) -p 'Project@/litar@/Build@/commands'

sync: $(EVALUATOR)
	$(EVAL) -p 'Project@/litar@/Build@/source files'

mod: $(EVALUATOR)
	$(EVAL) -p 'Language@/Go@/Module@/module file report'

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

define install_plugin
	rm -rf "$(1)"
	install -d "$(1)"
	cp -RL "$(PLUGIN)/." "$(1)/"
	find "$(1)" -name __pycache__ -prune -exec rm -rf {} +
endef

install-cursor:
	$(call install_plugin,$(CURSOR_DIR)/plugins/local/litar)

install-claude:
	@command -v claude >/dev/null 2>&1 || { echo "claude is not on PATH" >&2; exit 1; }
	$(call install_plugin,$(CLAUDE_DIR)/local-marketplaces/litar/litar)
	install -d "$(CLAUDE_DIR)/local-marketplaces/litar/.claude-plugin"
	printf '%s\n' \
		'{' \
		'  "name": "litar",' \
		'  "owner": { "name": "Chu" },' \
		'  "plugins": [{ "name": "litar", "source": "./litar" }]' \
		'}' > "$(CLAUDE_DIR)/local-marketplaces/litar/.claude-plugin/marketplace.json"
	claude plugin marketplace remove litar >/dev/null 2>&1 || true
	claude plugin marketplace add "$(abspath $(CLAUDE_DIR)/local-marketplaces/litar)"
	claude plugin install litar@litar

install-antigravity:
	$(call install_plugin,$(ANTIGRAVITY_DIR)/plugins/litar)

mcp:
	npx @modelcontextprotocol/inspector

clean:
	rm -rf $(BIN) $(dir $(BOOT))
