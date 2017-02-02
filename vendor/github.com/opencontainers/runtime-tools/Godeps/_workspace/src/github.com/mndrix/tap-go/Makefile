TESTS = auto check diagnostic known failing writer
GOPATH = $(CURDIR)/gopath

.PHONY: $(TESTS)

all: $(foreach t,$(TESTS),test/$(t)/test)
	prove -v -e '' test/*/test

clean:
	rm -f test/*/test

test/%/test: test/%/main.go tap.go
	go build -o $@ $<

$(TESTS): %: test/%/test
	prove -v -e '' test/$@/test
