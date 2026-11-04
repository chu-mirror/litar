# Build litar from the composition in compos, sync src from it, and install.
#   make                  evaluate the go module files into $(BUILD), then compile ./litar
#   make sync             write every file of the composition into $(SRC)
#   make boot             compile $(BOOT) from the Go sources in $(SRC)
#   make install          install ./litar and the grounds under $(PREFIX)
#   make install-bin      install ./litar as $(BINDIR)/litar
#   make install-grounds  copy each ground composition into $(GROUNDDIR)
#   make clean            remove $(BUILD), $(BOOT), and ./litar
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

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
GROUNDDIR ?= $(PREFIX)/share/litar

ARCHIVE := compos/litar.la
GROUNDS := grounds
SRC ?= src
BUILD ?= tmp/build
BOOT ?= tmp/boot/litar
LISTING := tmp/litar-files

LITAR ?= $(or $(wildcard $(CURDIR)/litar),$(abspath $(BOOT)))

GROUND_NAMES := $(notdir $(patsubst %/composer.json,%,$(wildcard $(GROUNDS)/*/composer.json)))
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

.PHONY: all litar tangle sync boot install install-bin install-grounds clean

all: litar

boot: $(abspath $(BOOT))

$(abspath $(BOOT)): $(BOOT_SOURCES)
	mkdir -p $(dir $@)
	cd $(SRC) && GOWORK=off go build -o $@ ./cmd/litar

tangle: $(EVALUATOR)
	rm -rf $(BUILD)
	$(call write-files,Build@/go module files,$(BUILD))

litar: tangle
	cd $(BUILD) && GOWORK=off go build -o $(CURDIR)/litar ./cmd/litar

sync: $(EVALUATOR)
	$(call write-files,Build@/project files,$(SRC))

install: install-bin install-grounds

install-bin:
	@test -x litar || { echo "./litar is missing; run make first" >&2; exit 1; }
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 litar "$(DESTDIR)$(BINDIR)/litar"

install-grounds:
	for g in $(GROUND_NAMES); do \
		rm -rf "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		install -d "$(DESTDIR)$(GROUNDDIR)/$$g" && \
		cp -R "$(GROUNDS)/$$g/." "$(DESTDIR)$(GROUNDDIR)/$$g/" || exit 1; \
	done

mcp:
	npx @modelcontextprotocol/inspector

clean:
	rm -rf $(BUILD) $(dir $(BOOT)) $(LISTING) litar

