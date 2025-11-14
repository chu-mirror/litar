PREFIX=/usr/local
EXEC=stage0.perl

all: litar

.SUFFIXES: .la
.la.c:
	litar $@ $< > $@

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la
	./stage0.perl $@ $< > $@.bak
	indent $@.bak -o $@
	rm $@.bak

clean:
	rm -f litar.c litar hello litar.1

