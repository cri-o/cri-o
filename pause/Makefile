src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
CFLAGS ?= -std=c99 -Os -Wall -Wextra -static

../bin/pause: $(obj) | ../bin
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

../bin:
	mkdir -p $@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pause
