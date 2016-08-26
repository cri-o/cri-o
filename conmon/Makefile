src = $(wildcard *.c)
obj = $(src:.c=.o)

LIBS = $(shell pkg-config --libs glib-2.0)
CFLAGS = -Wall -Wextra $(shell pkg-config --cflags glib-2.0)

conmon: $(obj)
	$(CC) -o $@ $^ $(LIBS)

.PHONY: clean
clean:
	rm -f $(obj) conmon
