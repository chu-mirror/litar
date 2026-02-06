PREFIX=/usr/local
EXEC=litar

CFLAGS=-g -Ilight/include `pkg-config fuse3 --cflags --libs`
LIBS=`pkg-config fuse3 --libs`

LIGHT_HEADER=context.h list.h function.h higher.h
LIGHT_SOURCE=context.c list.c function.c higher.c

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

hello: examples/hello.la litar
	./litar -p "hello.c" examples/hello.la > hello.c
	./litar -p "hello.h" examples/hello.la > hello.h
	./litar -p "main.c" examples/hello.la > main.c
	gcc -o hello main.c hello.c

$(LIGHT_HEADER) $(LIGHT_SOURCE): lib/light.la litar
	./litar -m Light -p $@ lib/light.la > $@

show.c: examples/show-light.la $(LIGHT_HEADER)
	./litar -p "show.c" examples/show-light.la > show.c

show: show.c $(LIGHT_SOURCE)
	gcc -g -o $@ $^

clean:
	rm -f litar.c litar hello litar.1 boot hello.c hello.h main.c $(LIGHT_HEADER) $(LIGHT_SOURCE)

