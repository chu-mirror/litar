PREFIX=/usr/local
EXEC=stage0.perl

all: litar

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la
	./stage0.perl "litar.c" litar.la > litar.c

clean:
	rm -f litar.c litar
