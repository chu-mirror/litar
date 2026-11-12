# Run the acceptance cases with the binary that `make` builds at the repo root.

LITAR ?= ../../bin/litar

.PHONY: all test clean

all: test

test:
	bash run.sh $(abspath $(LITAR))

clean:
	rm -rf tmp
