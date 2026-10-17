# Run the test set. From the repo root, `make test` builds first.
# `make -C tests` builds ../litar when it is missing or older than src/litar.c.

ROOT := ..
LITAR ?= $(ROOT)/litar

.PHONY: all test clean

all: test

test: $(LITAR)
	bash run.sh $(abspath $(LITAR))

$(LITAR): $(ROOT)/src/litar.c
	$(MAKE) -C $(ROOT) litar

clean:
	rm -rf tmp
