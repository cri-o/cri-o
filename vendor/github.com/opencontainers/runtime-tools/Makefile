PREFIX ?= $(DESTDIR)/usr
BINDIR ?= $(DESTDIR)/usr/bin

BUILDTAGS=
RUNTIME_TOOLS_LINK := $(CURDIR)/Godeps/_workspace/src/github.com/opencontainers/runtime-tools
export GOPATH:=$(CURDIR)/Godeps/_workspace:$(GOPATH)

all: $(RUNTIME_TOOLS_LINK)
	go build -tags "$(BUILDTAGS)" -o oci-runtime-tool ./cmd/oci-runtime-tool
	go build -tags "$(BUILDTAGS)" -o runtimetest ./cmd/runtimetest

.PHONY: man
man:
	go-md2man -in "man/oci-runtime-tool.1.md" -out "oci-runtime-tool.1"
	go-md2man -in "man/oci-runtime-tool-generate.1.md" -out "oci-runtime-tool-generate.1"
	go-md2man -in "man/oci-runtime-tool-validate.1.md" -out "oci-runtime-tool-validate.1"

install: man
	install -d -m 755 $(BINDIR)
	install -m 755 oci-runtime-tool $(BINDIR)
	install -d -m 755 $(PREFIX)/share/man/man1
	install -m 644 *.1 $(PREFIX)/share/man/man1
	install -d -m 755 $(PREFIX)/share/bash-completion/completions
	install -m 644 completions/bash/oci-runtime-tool $(PREFIX)/share/bash-completion/completions

uninstall:
	rm -f $(BINDIR)/oci-runtime-tool
	rm -f $(PREFIX)/share/man/man1/oci-runtime-tool*.1
	rm -f $(PREFIX)/share/bash-completion/completions/oci-runtime-tool

clean:
	rm -f oci-runtime-tool runtimetest *.1
	rm -f $(RUNTIME_TOOLS_LINK)

$(RUNTIME_TOOLS_LINK):
	ln -sf $(CURDIR) $(RUNTIME_TOOLS_LINK)

.PHONY: test .gofmt .govet .golint

test: .gofmt .govet .golint

.gofmt:
	go fmt ./...

.govet:
	go vet -x ./...

.golint:
	golint ./...

