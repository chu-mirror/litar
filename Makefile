# Build the litar binary and run the test set.
#   make        compile ./litar from src/litar.c
#   make test   compile, then run tests/
#   make clean  remove the binary

CC ?= gcc
CFLAGS ?= -std=c11 -Wall -Wextra -Werror

.PHONY: all test clean plugin grok

all: litar

litar: src/litar.c
	$(CC) $(CFLAGS) -o $@ src/litar.c

test: litar
	$(MAKE) -C tests test LITAR=$(abspath litar)

clean:
	rm -f litar
	$(MAKE) -C tests clean

