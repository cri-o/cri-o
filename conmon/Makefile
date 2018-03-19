include ../Makefile.inc

all: ../bin/conmon

src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS += $(shell pkg-config --libs glib-2.0)

VERSION = $(shell sed -n -e 's/^const Version = "\([^"]*\)"/\1/p' ../version/version.go)

override CFLAGS += -std=c99 -Os -Wall -Wextra $(shell pkg-config --cflags glib-2.0) -DVERSION=\"$(VERSION)\" -DGIT_COMMIT=\"$(GIT_COMMIT)\"

config.h: ../oci/oci.go
	$(MAKE) -C .. conmon/config.h

../bin/conmon: config.h $(obj)
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

.PHONY: clean
clean:
	rm -f $(obj) ../bin/conmon
	rm -f config.h
