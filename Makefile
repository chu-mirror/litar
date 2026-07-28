PREFIX=/usr/local
EXEC=litar

CFLAGS=-g `pkg-config fuse3 --cflags --libs` \
    -Dlight_option_context_main_size="(1<<23)" \
    -Dlight_option_logging_level=INFO
LIBS=`pkg-config fuse3 --libs`

LIGHT_HEADER=light.h state.h
LIGHT_SOURCE=light.c state.c

STATIC_LIGHT_HEADER=_light.h
STATIC_LIGHT_SOURCE=_light.c

all: litar

.SUFFIXES: .la
.la.c:
	./litar -p "$@" $< > $@

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar

litar.c: litar.la boot
	./boot -p "$@" litar.la > $@

litarfs.c: litar.la boot
	./boot -p "$@" litar.la > $@

_litar.c: _litar.la boot
	./boot -p "litar.c" _litar.la > $@

litar.1: litar
	./litar -p $@ litar.la > $@

boot: boot.c
	$(CC) $(CFLAGS) -Ilight/include -o $@ $< `./light/finddeff.py $< ./light/include` $(LIBS)

litar: litar.c $(LIGHT_SOURCE) $(LIGHT_HEADER)
	$(CC) $(CFLAGS) -o $@ $< $(LIGHT_SOURCE) $(LIBS)

litarfs: litarfs.c $(LIGHT_SOURCE) $(LIGHT_HEADER)
	$(CC) $(CFLAGS) -o $@ $< $(LIGHT_SOURCE) $(LIBS)

release: litar
	./litar -p "README.md" litar.la > README.md

hello: examples/hello.la litar
	./litar -p "hello.c" examples/hello.la > hello.c
	./litar -p "hello.h" examples/hello.la > hello.h
	./litar -p "main.c" examples/hello.la > main.c
	gcc -o hello main.c hello.c

$(LIGHT_HEADER) $(LIGHT_SOURCE): lib/light.la boot
	./boot -m Light -p $@ litar.la > $@

$(STATIC_LIGHT_HEADER) $(STATIC_LIGHT_SOURCE): lib/_light.la boot
	./boot -m Light -p $@ lib/_light.la > $@

show.c: examples/show-light.la $(LIGHT_HEADER)
	./boot -p "show.c" examples/show-light.la > show.c

show: show.c $(LIGHT_SOURCE)
	gcc -g -o $@ $^ -pthread

test-light.c: examples/test-light.la $(STATIC_LIGHT_HEADER)
	./boot -p "test-light.c" examples/test-light.la > test-light.c

test-light: test-light.c $(STATIC_LIGHT_SOURCE)
	gcc -Dlight_option_context_main_size="(1<<23)" -Dlight_option_logging_level=INFO -g -o $@ $^ -pthread

_litar: _litar.c $(STATIC_LIGHT_SOURCE) $(STATIC_LIGHT_HEADER)
	$(CC) $(CFLAGS) -o $@ $< $(STATIC_LIGHT_SOURCE) $(LIBS)

clean:
	rm -rf litar.c litar hello litar.1 boot hello.c hello.h show show.c main.c \
	    $(LIGHT_HEADER) $(LIGHT_SOURCE) litarfs litarfs.c

