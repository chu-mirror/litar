PREFIX=/usr/local
EXEC=litar

CFLAGS=-g -Ilight/include `pkg-config fuse3 --cflags --libs`
LIBS=`pkg-config fuse3 --libs`

all: litar

.SUFFIXES: .la
.la.c:
	litar $@ $< > $@

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la boot
	./boot -p "$@" litar.la > $@
	clang-format-19 -style=file -i $@

boot: boot.c
	$(CC) $(CFLAGS) -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

litar: litar.c
	$(CC) $(CFLAGS) -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

clean:
	rm -f litar.c litar hello litar.1 boot

