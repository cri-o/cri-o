all: git-vars ../bin/conmon

include ../Makefile.inc

src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS += $(shell pkg-config --libs glib-2.0)

VERSION = $(shell sed -n -e 's/^const Version = "\([^"]*\)"/\1/p' ../version/version.go)

CFLAGS ?= -std=c99 -Os -Wall -Wextra
override CFLAGS += $(shell pkg-config --cflags glib-2.0) -DVERSION=\"$(VERSION)\" -DGIT_COMMIT=\"$(GIT_COMMIT)\"

# Conditionally compile journald logging code if the libraries can be found
# if they can be found, set USE_JOURNALD macro for use in conmon code.
#
# "pkg-config --exists" will error if the package doesn't exist. Make can only compare
# output of commands, so the echo commands are to allow pkg-config to error out, make to catch it,
# and allow the compilation to complete.
ifeq ($(shell pkg-config --exists libsystemd-journal && echo "0" || echo "1"), 0)
	override LIBS += $(shell pkg-config --libs libsystemd-journal)
	override CFLAGS += $(shell pkg-config --cflags libsystemd-journal) -D USE_JOURNALD=0
else ifeq ($(shell pkg-config --exists libsystemd && echo "0" || echo "1"), 0)
	override LIBS += $(shell pkg-config --libs libsystemd)
	override CFLAGS += $(shell pkg-config --cflags libsystemd) -D USE_JOURNALD=0
endif

config.h: ../oci/oci.go
	$(MAKE) -C .. conmon/config.h

../bin/conmon: config.h $(obj) | ../bin
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

../bin:
	mkdir -p $@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/conmon
	rm -f config.h
