# Build litar from the composition in compos, sync src from it, and install.
#   make                  evaluate the go module files into $(BUILD), then compile ./litar and ./litar-web
#   make sync             write every file of the composition into $(SRC)
#   make boot             compile $(BOOT) from the Go sources in $(SRC)
#   make web              browse $(ARCHIVE) with ./litar-web on $(WEB_ADDR)
#   make install          install ./litar, ./litar-web, and the grounds under $(PREFIX)
#   make install-bin      install ./litar and ./litar-web into $(BINDIR)
#   make install-grounds  copy each ground composition into $(GROUNDDIR)
#   make install-cursor   install the MCP server and the agent skills for Cursor under $(CURSOR_DIR)
#   make install-mcp      add the litar server, run as $(BINDIR)/litar, to $(MCP_CONFIG)
#   make install-skills   copy each directory of $(SKILLS) into $(CURSOR_DIR)/skills
#   make clean            remove $(BUILD), $(BOOT), ./litar, and ./litar-web
#
# LITAR is the binary that evaluates the composition: ./litar from an earlier build,
# else $(BOOT), which is compiled from $(SRC) first. So a fresh checkout needs only go,
# and `make sync` keeps $(SRC) able to boot the next build.
# Evaluation reads the grounds from this repository, not from the installed ones.
#
# sync overwrites the files the composition lists and leaves any other file in $(SRC).
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

ARCHIVE := compos/litar.la
GROUNDS := grounds
SRC ?= src
BUILD ?= tmp/build
BOOT ?= tmp/boot/litar
WEB_ADDR ?= 127.0.0.1:8080
COMMANDS := litar litar-web
LISTING := tmp/litar-files
CURSOR_DIR ?= $(HOME)/.cursor
MCP_CONFIG ?= $(CURSOR_DIR)/mcp.json
SKILLS := agent/skills

LITAR ?= $(or $(wildcard $(CURDIR)/litar),$(abspath $(BOOT)))

GROUND_NAMES := $(notdir $(patsubst %/composer.json,%,$(wildcard $(GROUNDS)/*/composer.json)))
SKILL_NAMES := $(notdir $(wildcard $(SKILLS)/*))
BOOT_SOURCES := $(SRC)/go.mod $(SRC)/go.sum $(shell find $(SRC)/cmd $(SRC)/internal -name '*.go' 2>/dev/null)
EVAL = LITAR_LIBRARY=$(abspath $(GROUNDS)) $(LITAR) -f $(ARCHIVE)
# Build the bootstrap binary only when it is the one evaluating.
EVALUATOR := $(filter $(abspath $(BOOT)),$(LITAR))

# $(call write-files,LISTING CHUNK,DIRECTORY): evaluate each listed file into DIRECTORY.
# A listing line is x (executable) or - (regular), a space, and the path.
# A file is written beside its target and renamed, so a failed evaluation keeps the old file.
# @ in a path is doubled, because -p reads a name.
define write-files
mkdir -p $(dir $(LISTING))
$(EVAL) -p '$(1)' > $(LISTING)
while read -r kind f; do \
	name=$$(printf '%s' "$$f" | sed 's/@/@@/g'); \
	mkdir -p "$(2)/$$(dirname "$$f")" && \
	$(EVAL) -p "$$name" > "$(2)/$$f.litar-new" && \
	mv "$(2)/$$f.litar-new" "$(2)/$$f" && \
	{ [ "$$kind" != x ] || chmod +x "$(2)/$$f"; } || \
	{ rm -f "$(2)/$$f.litar-new"; exit 1; }; \
done < $(LISTING)
endef

# Set the litar entry of the MCP configuration in argv[1] to run argv[2] with the grounds in argv[3].
define MCP_MERGE
import json, os, sys
path, litar, grounds = sys.argv[1:]
try:
    with open(path) as f:
        text = f.read()
except FileNotFoundError:
    text = ''
config = json.loads(text) if text.strip() else {}
config.setdefault('mcpServers', {})['litar'] = {
    'command': litar, 'args': ['-a'], 'env': {'LITAR_LIBRARY': grounds}}
os.makedirs(os.path.dirname(os.path.abspath(path)), exist_ok=True)
with open(path + '.litar-new', 'w') as f:
    json.dump(config, f, indent=2)
    f.write('\n')
os.replace(path + '.litar-new', path)
endef
export MCP_MERGE

.PHONY: all litar tangle sync boot web install install-bin install-grounds
.PHONY: install-cursor install-mcp install-skills mcp clean

all: litar

boot: $(abspath $(BOOT))

$(abspath $(BOOT)): $(BOOT_SOURCES)
	mkdir -p $(dir $@)
	cd $(SRC) && GOWORK=off go build -o $@ ./cmd/litar

tangle: $(EVALUATOR)
	rm -rf $(BUILD)
	$(call write-files,Build@/go module files,$(BUILD))

litar: tangle
	cd $(BUILD) && for c in $(COMMANDS); do GOWORK=off go build -o $(CURDIR)/$$c ./cmd/$$c || exit 1; done

sync: $(EVALUATOR)
	$(call write-files,Build@/project files,$(SRC))

web:
	@test -x litar-web || { echo "./litar-web is missing; run make first" >&2; exit 1; }
	LITAR_LIBRARY=$(abspath $(GROUNDS)) ./litar-web -addr $(WEB_ADDR) -litar $(CURDIR)/litar -file $(ARCHIVE)

install: install-bin install-grounds

install-bin:
	@for c in $(COMMANDS); do test -x $$c || { echo "./$$c is missing; run make first" >&2; exit 1; }; done
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 $(COMMANDS) "$(DESTDIR)$(BINDIR)/"

install-grounds:
	for g in $(GROUND_NAMES); do \
		rm -rf "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		install -d "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		cp -R "$(GROUNDS)/$$g/." "$(DESTDIR)$(GROUNDDIR)/$$g/" || exit 1; \
	done

install-cursor: install-mcp install-skills

install-mcp:
	@test -x "$(abspath $(BINDIR))/litar" || { echo "$(abspath $(BINDIR))/litar is missing; run make install first" >&2; exit 1; }
	python3 -c "$$MCP_MERGE" "$(MCP_CONFIG)" "$(abspath $(BINDIR))/litar" "$(abspath $(GROUNDDIR))"

install-skills:
	for s in $(SKILL_NAMES); do \
		rm -rf "$(CURSOR_DIR)/skills/$$s" && \
		install -d "$(CURSOR_DIR)/skills/$$s" && \
		cp -R "$(SKILLS)/$$s/." "$(CURSOR_DIR)/skills/$$s/" || exit 1; \
	done

mcp:
	npx @modelcontextprotocol/inspector

clean:
	rm -rf $(BUILD) $(dir $(BOOT)) $(LISTING) $(COMMANDS)

