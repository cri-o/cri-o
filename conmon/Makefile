include ../Makefile.inc

all: conmon

src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS += $(shell pkg-config --libs glib-2.0)

VERSION = $(shell sed -n -e 's/^const Version = "\([^"]*\)"/\1/p' ../version/version.go)

override CFLAGS += -std=c99 -Os -Wall -Wextra $(shell pkg-config --cflags glib-2.0) -DVERSION=\"$(VERSION)\" -DGIT_COMMIT=\"$(GIT_COMMIT)\"

config.h: config.go ../oci/oci.go
	go run config.go

conmon: config.h $(obj)
	$(CC) -o ../bin/$@ $^ $(CFLAGS) $(LIBS)

.PHONY: clean
clean:
	rm -f $(obj) ../bin/conmon
	rm -f config.h
