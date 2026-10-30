# Run the acceptance cases. From the repo root, `make test` builds first.

ROOT := ..
LITAR ?= $(ROOT)/litar

.PHONY: all test clean

all: test

test:
	$(MAKE) -C $(ROOT) litar
	bash run.sh $(abspath $(LITAR))

clean:
	rm -rf tmp
