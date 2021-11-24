src = $(wildcard src/*.c)
obj = $(src:.c=.o)

STRIP ?= strip
override LIBS +=
CFLAGS ?= -std=c99 -Os -Wall -Werror -Wextra -static

all: ../bin/pinns

../bin/pinns: $(obj) | ../bin
	$(CC) -o $@ $^ $(CFLAGS) $(LDFLAGS) $(LIBS)
	$(STRIP) -s $@

%.o: %.c $(HEADERS)
	$(CC) $(CFLAGS) -O3 -o $@ -c $<

../bin:
	mkdir -p $@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pinns
