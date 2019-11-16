src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
CFLAGS ?= -std=c99 -Os -Wall -Wextra
override CFLAGS += -static

../bin/pinns: $(obj) | ../bin
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

../bin:
	mkdir -p $@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pinns
