GO ?= go
EPOCH_TEST_COMMIT ?= 78aae688e2932f0cfc2a23e28ad30b58c6b8577f
PROJECT = github.com/kubernetes-incubator/cri-o
GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN = $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
OCID_IMAGE = ocid_dev$(if $(GIT_BRANCH_CLEAN),:$(GIT_BRANCH_CLEAN))
OCID_LINK = ${CURDIR}/vendor/src/github.com/kubernetes-incubator/cri-o
OCID_LINK_DIR = ${CURDIR}/vendor/src/github.com/kubernetes-incubator
OCID_INSTANCE = ocid_dev
SYSTEM_GOPATH = ${GOPATH}
PREFIX ?= ${DESTDIR}/usr
BINDIR ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR ?= ${PREFIX}/share/man
ETCDIR ?= ${DESTDIR}/etc
ETCDIR_OCID ?= ${ETCDIR}/ocid
GO_MD2MAN ?= $(shell which go-md2man)
export GOPATH = ${CURDIR}/vendor
BUILDTAGS = $(shell hack/btrfs_tag.sh) $(shell hack/libdm_tag.sh)
BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions

if USE_SELINUX
BUILDTAGS += selinux
endif

if USE_SECCOMP
BUILDTAGS += seccomp
endif

if USE_APPARMOR
BUILDTAGS += apparmor
endif

all: binaries ocid.conf docs

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'binaries' - Build ocid, conmon and ocic"
	@echo " * 'integration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'gofmt' - Verify the source code gofmt"

lint: ${OCID_LINK}
	@which gometalinter > /dev/null 2>/dev/null || (echo "ERROR: gometalinter not found. Consider 'make install.tools' target" && false)
	@echo "checking lint"
	@./.tool/lint

${OCID_LINK}:
	mkdir -p ${OCID_LINK_DIR}
	ln -sfn ${CURDIR} ${OCID_LINK}

gofmt:
	@./hack/verify-gofmt.sh

conmon:
	make -C $@

pause:
	make -C $@

GO_SRC =  $(shell find . -name \*.go)
ocid: $(GO_SRC) | ${OCID_LINK}
	$(GO) build --tags "$(BUILDTAGS)" -o $@ ./cmd/server/

ocic: $(GO_SRC) | ${OCID_LINK}
	$(GO) build -o $@ ./cmd/client/

kpod: $(GO_SRC) | ${OCID_LINK}
	$(GO) build -o $@ ./cmd/kpod/

ocid.conf: ocid
	./ocid --config="" config --default > ocid.conf

clean-local:
	rm -f ocid.conf
	rm -f ocic ocid
	rm -f kpod
	rm -f ${OCID_LINK}
	rm -f docs/*.1 docs/*.5 docs/*.8
	find . -name \*~ -delete
	find . -name \#\* -delete
	make -C conmon clean
	make -C pause clean

ocidimage:
	docker build -t ${OCID_IMAGE} .

dbuild: ocidimage
	docker run --name=${OCID_INSTANCE} --privileged ${OCID_IMAGE} -v ${PWD}:/go/src/${PROJECT} --rm make binaries

integration: ocidimage
	docker run -e TESTFLAGS -e TRAVIS -t --privileged --rm -v ${CURDIR}:/go/src/${PROJECT} ${OCID_IMAGE} make localintegration

localintegration: binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: ocid ocic kpod conmon pause

MANPAGES_MD = $(wildcard docs/*.md)
MANPAGES    = $(MANPAGES_MD:%.md=%)

docs/%.1: docs/%.1.md
	@which go-md2man > /dev/null 2>/dev/null || (echo "ERROR: go-md2man not found. Consider 'make install.tools' target" && false)
	$(GO_MD2MAN) -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs/%.5: docs/%.5.md
	@which go-md2man > /dev/null 2>/dev/null || (echo "ERROR: go-md2man not found. Consider 'make install.tools' target" && false)
	$(GO_MD2MAN) -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs/%.8: docs/%.8.md
	@which go-md2man > /dev/null 2>/dev/null || (echo "ERROR: go-md2man not found. Consider 'make install.tools' target" && false)
	$(GO_MD2MAN) -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs: $(MANPAGES)

install-exec-local:
	install -D -m 755 kpod $(BINDIR)/kpod
	install -D -m 755 ocid $(BINDIR)/ocid
	install -D -m 755 ocic $(BINDIR)/ocic
	install -D -m 755 conmon/conmon $(LIBEXECDIR)/ocid/conmon
	install -D -m 755 pause/pause $(LIBEXECDIR)/ocid/pause
	install -d -m 755 $(MANDIR)/man{1,5,8}
	install -m 644 $(filter %.1,$(MANPAGES)) -t $(MANDIR)/man1
	install -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8
	install -D -m 644 ocid.conf $(ETCDIR_OCID)/ocid.conf
	install -D -m 644 seccomp.json $(ETCDIR_OCID)/seccomp.json

install.completions:
	install -d -m 755 ${BASHINSTALLDIR}
	install -m 644 -D completions/bash/kpod ${BASHINSTALLDIR}

install.systemd:
	install -D -m 644 contrib/systemd/ocid.service $(PREFIX)/lib/systemd/system/ocid.service

uninstall-local:
	rm -f $(BINDIR)/{ocid,ocic}
	rm -f $(LIBEXECDIR)/ocid/{conmon,pause}
	for i in $(filter %.1,$(MANPAGES)); do \
		rm -f $(MANDIR)/man8/$$(basename $${i}); \
	done
	for i in $(filter %.5,$(MANPAGES)); do \
		rm -f $(MANDIR)/man5/$$(basename $${i}); \
	done
	for i in $(filter %.8,$(MANPAGES)); do \
		rm -f $(MANDIR)/man8/$$(basename $${i}); \
	done

.PHONY: .gitvalidation
# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make install.tools' target" && false)
if TRAVIS
	git-validation -q -run DCO,short-subject
else
	git-validation -v -run DCO,short-subject -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: install.tools

install.tools: .install.gitvalidation .install.gometalinter .install.md2man

.install.gitvalidation:
	GOPATH=${SYSTEM_GOPATH} go get github.com/vbatts/git-validation

.install.gometalinter:
	GOPATH=${SYSTEM_GOPATH} go get github.com/alecthomas/gometalinter
	GOPATH=${SYSTEM_GOPATH} gometalinter --install

.install.md2man:
	GOPATH=${SYSTEM_GOPATH} go get github.com/cpuguy83/go-md2man

.PHONY: \
	binaries \
	clean \
	conmon \
	default \
	docs \
	gofmt \
	help \
	install \
	lint \
	pause \
	uninstall
