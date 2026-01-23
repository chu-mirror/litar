PREFIX=/usr/local
EXEC=litar

CFLAGS=-g -Ilight/include `pkg-config fuse3 --cflags --libs`
LIBS=`pkg-config fuse3 --libs`

all: litar

.SUFFIXES: .la
.la.c:
	./litar -p "$@" $< > $@

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la boot
	./boot -p "$@" litar.la > $@
	clang-format-19 -style=file -i $@

litar.1: litar
	./litar -p "manpage of litar" litar.la > $@

boot: boot.c
	$(CC) $(CFLAGS) -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

litar: litar.c
	$(CC) $(CFLAGS) -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

release: litar
	./litar -p "README.md" litar.la > README.md

clean:
	rm -f litar.c litar hello litar.1 boot

