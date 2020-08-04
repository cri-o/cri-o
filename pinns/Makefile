src = $(wildcard *.c)
obj = $(src:.c=.o)

STRIP ?= strip
override LIBS +=
CFLAGS ?= -std=c99 -Os -Wall -Werror -Wextra
override CFLAGS += -static

all: ../bin/pinns

../bin/pinns: $(obj) | ../bin
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)
	$(STRIP) -s $@

../bin:
	mkdir -p $@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pinns
