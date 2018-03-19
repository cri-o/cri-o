src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
override CFLAGS += -std=c99 -Os -Wall -Wextra -static

../bin/pause: $(obj)
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pause
