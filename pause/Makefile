src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
override CFLAGS += -Os -Wall -Wextra -static

pause: $(obj)
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)
	strip $@

.PHONY: clean
clean:
	rm -f $(obj) pause
