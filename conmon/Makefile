src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS += $(shell pkg-config --libs glib-2.0)
override CFLAGS += -Wall -Wextra $(shell pkg-config --cflags glib-2.0)

conmon: $(obj)
	$(CC) -o $@ $^ $(LIBS)

.PHONY: clean
clean:
	rm -f $(obj) conmon
