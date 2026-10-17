# Build the litar binary and run the test set.
#   make        compile ./litar from src/litar.c
#   make tangle regenerate src/litar.c from src/litar.la
#   make test   compile, then run tests/
#   make clean  remove the binary

CC ?= gcc
CFLAGS ?= -std=c11 -Wall -Wextra -Werror

.PHONY: all test clean plugin grok tangle

all: litar

litar: src/litar.c
	$(CC) $(CFLAGS) -o $@ src/litar.c

# src/litar.la is the literate source. Tangling needs a litar binary,
# so this builds the current translation unit first, then replaces it.
tangle: litar
	./litar -p litar.c src/litar.la > src/litar.c.tmp && mv src/litar.c.tmp src/litar.c || { rm -f src/litar.c.tmp; exit 1; }

test: litar
	$(MAKE) -C tests test LITAR=$(abspath litar)

clean:
	rm -f litar
	$(MAKE) -C tests clean

plugin: litar
	[ -d plugin/bin ] || mkdir -p plugin/bin && cp src/mcp.py litar plugin/bin

grok:
	grok mcp remove litar || true
	grok mcp add --scope project litar -- ./src/mcp.py

