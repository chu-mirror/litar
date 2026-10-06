# Build the stage 0 litar binary and run the test set.
#   make        compile ./litar from src/stage0.c
#   make test   compile, then run tests/
#   make clean  remove the binary

CC ?= gcc
CFLAGS ?= -std=c11 -Wall -Wextra -Werror

.PHONY: all test clean

all: litar

litar: src/stage0.c
	$(CC) $(CFLAGS) -o $@ src/stage0.c

test: litar
	$(MAKE) -C tests test LITAR=$(abspath litar)

clean:
	rm -f litar
	$(MAKE) -C tests clean
