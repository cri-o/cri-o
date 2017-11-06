src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
override CFLAGS += -std=c99 -Os -Wall -Wextra -static

pause: $(obj)
	$(CC) -o ../bin/$@ $^ $(CFLAGS) $(LIBS)
ifndef DISTRO_BUILD
	strip ../bin/$@
endif

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pause
