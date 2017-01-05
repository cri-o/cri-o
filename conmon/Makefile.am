src = $(wildcard *.c)
obj = $(src:.c=.o)

conmon: $(obj)
	$(CC) -o $@ $^ $(CFLAGS) $(LIBS)

.PHONY: clean
clean-local:
	rm -f $(obj) conmon
