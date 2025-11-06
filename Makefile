PREFIX=/usr/local
EXEC=stage0.perl

install:
	install -C -m 755 ${EXEC} ${PREFIX}/bin/litar
