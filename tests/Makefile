# Run the stage 0 test set. From the repo root, `make test` builds first.
# `make -C tests` builds ../litar when it is missing or older than src/stage0.c.

ROOT := ..
LITAR ?= $(ROOT)/litar

.PHONY: all test clean

all: test

test: $(LITAR)
	bash run.sh $(abspath $(LITAR))

$(LITAR): $(ROOT)/src/stage0.c
	$(MAKE) -C $(ROOT) litar

clean:
	rm -rf tmp
