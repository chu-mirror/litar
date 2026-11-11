# Build the litar binary and run the test set.
#   make        compile ./litar
#   make test   run the acceptance cases of tests/
#   make clean  remove the binary

.PHONY: all test clean litar

all: litar

litar:
	go build -o litar ./cmd/litar

test: litar
	bash tests/run.sh $(abspath litar)

clean:
	rm -f litar
	$(MAKE) -C tests clean

mcp:
	npx @modelcontextprotocol/inspector

