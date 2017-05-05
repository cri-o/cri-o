GO ?= go
EPOCH_TEST_COMMIT ?= 78aae688e2932f0cfc2a23e28ad30b58c6b8577f
PROJECT := github.com/kubernetes-incubator/cri-o
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
OCID_IMAGE := ocid_dev$(if $(GIT_BRANCH_CLEAN),:$(GIT_BRANCH_CLEAN))
OCID_INSTANCE := ocid_dev
PREFIX ?= ${DESTDIR}/usr/local
BINDIR ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR ?= ${PREFIX}/share/man
ETCDIR ?= ${DESTDIR}/etc
ETCDIR_OCID ?= ${ETCDIR}/ocid
BUILDTAGS := selinux seccomp $(shell hack/btrfs_tag.sh) $(shell hack/libdm_tag.sh)
BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions

# If GOPATH not specified, use one in the local directory
ifeq ($(GOPATH),)
export GOPATH := $(CURDIR)/_output
unexport GOBIN
endif
GOPKGDIR := $(GOPATH)/src/$(PROJECT)
GOPKGBASEDIR := $(shell dirname "$(GOPKGDIR)")

# Update VPATH so make finds .gopathok
VPATH := $(VPATH):$(GOPATH)

all: binaries ocid.conf docs

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations"
	@echo " * 'binaries' - Build ocid, conmon and ocic"
	@echo " * 'integration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'gofmt' - Verify the source code gofmt"

.gopathok:
ifeq ("$(wildcard $(GOPKGDIR))","")
	mkdir -p "$(GOPKGBASEDIR)"
	ln -s "$(CURDIR)" "$(GOPKGBASEDIR)"
endif
	touch "$(GOPATH)/.gopathok"

lint: .gopathok
	@echo "checking lint"
	@./.tool/lint

gofmt:
	@./hack/verify-gofmt.sh

conmon:
	$(MAKE) -C $@

pause:
	$(MAKE) -C $@

bin2img: .gopathok $(wildcard test/bin2img/*.go)
	go build -tags "$(BUILDTAGS)" -o test/bin2img/$@ $(PROJECT)/test/bin2img

copyimg: .gopathok $(wildcard test/copyimg/*.go)
	go build -tags "$(BUILDTAGS)" -o test/copyimg/$@ $(PROJECT)/test/copyimg

checkseccomp: .gopathok $(wildcard test/checkseccomp/*.go)
	go build -o test/checkseccomp/$@ $(PROJECT)/test/checkseccomp

ocid: .gopathok $(shell hack/find-godeps.sh $(GOPKGDIR) cmd/ocid $(PROJECT))
	$(GO) build -o $@ \
		-tags "$(BUILDTAGS)" \
		$(PROJECT)/cmd/ocid

ocic: .gopathok $(shell hack/find-godeps.sh $(GOPKGDIR) cmd/ocic $(PROJECT))
	$(GO) build -o $@ $(PROJECT)/cmd/ocic

kpod: .gopathok $(shell hack/find-godeps.sh $(GOPKGDIR) cmd/kpod $(PROJECT))
	$(GO) build -o $@ $(PROJECT)/cmd/kpod

ocid.conf: ocid
	./ocid --config="" config --default > ocid.conf

clean:
ifneq ($(GOPATH),)
	rm -f "$(GOPATH)/.gopathok"
endif
	rm -rf _output
	rm -f docs/*.1 docs/*.5 docs/*.8
	rm -fr test/testdata/redis-image
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -f ocic ocid kpod
	make -C conmon clean
	make -C pause clean
	rm -f test/bin2img/bin2img
	rm -f test/copyimg/copyimg
	rm -f test/checkseccomp/checkseccomp

ocidimage:
	docker build -t ${OCID_IMAGE} .

dbuild: ocidimage
	docker run --name=${OCID_INSTANCE} --privileged ${OCID_IMAGE} -v ${PWD}:/go/src/${PROJECT} --rm make binaries

integration: ocidimage
	docker run -e TESTFLAGS -e TRAVIS -t --privileged --rm -v ${CURDIR}:/go/src/${PROJECT} ${OCID_IMAGE} make localintegration

localintegration: binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: ocid ocic kpod conmon pause bin2img copyimg checkseccomp

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

docs/%.1: docs/%.1.md .gopathok
	go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@ || $(GOPATH)/bin/go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs/%.5: docs/%.5.md .gopathok
	go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@ || $(GOPATH)/bin/go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs/%.8: docs/%.8.md .gopathok
	go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@ || $(GOPATH)/bin/go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@

docs: $(MANPAGES)

install: .gopathok
	install -D -m 755 ocid $(BINDIR)/ocid
	install -D -m 755 ocic $(BINDIR)/ocic
	install -D -m 755 kpod $(BINDIR)/kpod
	install -D -m 755 conmon/conmon $(LIBEXECDIR)/ocid/conmon
	install -D -m 755 pause/pause $(LIBEXECDIR)/ocid/pause
	install -d -m 755 $(MANDIR)/man1
	install -d -m 755 $(MANDIR)/man5
	install -d -m 755 $(MANDIR)/man8
	install -m 644 $(filter %.1,$(MANPAGES)) -t $(MANDIR)/man1
	install -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8

install.config:
	install -D -m 644 ocid.conf $(ETCDIR_OCID)/ocid.conf
	install -D -m 644 seccomp.json $(ETCDIR_OCID)/seccomp.json

install.completions:
	install -d -m 755 ${BASHINSTALLDIR}
	install -m 644 -D completions/bash/kpod ${BASHINSTALLDIR}

install.systemd:
	install -D -m 644 contrib/systemd/ocid.service $(PREFIX)/lib/systemd/system/ocid.service
	install -D -m 644 contrib/systemd/ocid-shutdown.service $(PREFIX)/lib/systemd/system/ocid-shutdown.service

uninstall:
	rm -f $(BINDIR)/ocid
	rm -f $(BINDIR)/ocic
	rm -f $(LIBEXECDIR)/ocid/conmon
	rm -f $(LIBEXECDIR)/ocid/pause
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
.gitvalidation: .gopathok
ifeq ($(TRAVIS),true)
	$(GOPATH)/bin/git-validation -q -run DCO,short-subject
else
	$(GOPATH)/bin/git-validation -v -run DCO,short-subject -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: install.tools

install.tools: .install.gitvalidation .install.gometalinter .install.md2man

.install.gitvalidation: .gopathok
	if [ ! -x "$(GOPATH)/bin/git-validation" ]; then \
		go get -u github.com/vbatts/git-validation; \
	fi

.install.gometalinter: .gopathok
	if [ ! -x "$(GOPATH)/bin/gometalinter" ]; then \
		go get -u github.com/alecthomas/gometalinter; \
		$(GOPATH)/bin/gometalinter --install; \
	fi

.install.md2man: .gopathok
	if [ ! -x "$(GOPATH)/bin/go-md2man" ]; then \
		go get -u github.com/cpuguy83/go-md2man; \
	fi

.PHONY: \
	bin2img \
	binaries \
	checkseccomp \
	clean \
	conmon \
	copyimg \
	default \
	docs \
	gofmt \
	help \
	install \
	lint \
	pause \
	uninstall
