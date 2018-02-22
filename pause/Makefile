src = $(wildcard *.c)
obj = $(src:.c=.o)

override LIBS +=
override CFLAGS += -static

pause: $(obj)
	$(CC) -o ../bin/$@ $^ $(CFLAGS) $(LIBS)
	strip ../bin/$@

.PHONY: clean
clean:
	rm -f $(obj) ../bin/pause
