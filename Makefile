# Build the boot binary and run the test set.
#   make        compile ./litar from src/boot.c
#   make test   compile, then run tests/
#   make clean  remove the binary

CC ?= gcc
CFLAGS ?= -std=c11 -Wall -Wextra -Werror

.PHONY: all test clean plugin grok

all: litar

litar: src/boot.c
	$(CC) $(CFLAGS) -o $@ src/boot.c

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

