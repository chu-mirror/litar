PREFIX=/usr/local
EXEC=stage0.perl

CFLAGS=-g -Ilight/include `pkg-config fuse3 --cflags --libs`
LIBS=`pkg-config fuse3 --libs`

all: litar

.SUFFIXES: .la
.la.c:
	litar $@ $< > $@

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la
	./stage0.perl $@ $< > $@.bak
	indent -orig $@.bak -o $@ 
	rm $@.bak

litar: litar.c
	$(CC) $(CFLAGS) -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

clean:
	rm -f litar.c litar hello litar.1

