EPOCH_TEST_COMMIT ?= 78aae
PROJECT := github.com/kubernetes-incubator/cri-o
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
OCID_IMAGE := ocid_dev$(if $(GIT_BRANCH_CLEAN),:$(GIT_BRANCH_CLEAN))
OCID_LINK := ${CURDIR}/vendor/src/github.com/kubernetes-incubator/cri-o
OCID_LINK_DIR := ${CURDIR}/vendor/src/github.com/kubernetes-incubator
OCID_INSTANCE := ocid_dev
SYSTEM_GOPATH := ${GOPATH}
PREFIX ?= ${DESTDIR}/usr
INSTALLDIR=${PREFIX}/bin
GO_MD2MAN ?= $(shell which go-md2man)
export GOPATH := ${CURDIR}/vendor
BUILDTAGS := selinux

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'binaries' - Build ocid, conmon and ocic"
	@echo " * 'integration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"

lint: ${OCID_LINK}
	@which gometalinter > /dev/null 2>/dev/null || (echo "ERROR: gometalinter not found. Consider 'make install.tools' target" && false)
	@echo "checking lint"
	@./.tool/lint

${OCID_LINK}:
	mkdir -p ${OCID_LINK_DIR}
	ln -sfn ${CURDIR} ${OCID_LINK}

conmon:
	make -C $@

pause:
	make -C $@

ocid: ${OCID_LINK}
	go build --tags "$(BUILDTAGS)" -o ocid ./cmd/server/

ocic: ${OCID_LINK}
	go build -o ocic ./cmd/client/

ocid.conf: ocid
	 ./ocid --config="" config --default > ocid.conf 

clean:
	rm -f ocid.conf
	rm -f ocic ocid
	rm -f ${OCID_LINK}
	rm -f conmon/conmon.o conmon/conmon
	rm -f pause/pause.o pause/pause
	rm -f docs/*.1 docs/*.5{,.gz} docs/*.8{,.gz}
	find . -name \*~ -delete
	find . -name \#\* -delete

ocidimage:
	docker build -t ${OCID_IMAGE} .

dbuild: ocidimage
	docker run --name=${OCID_INSTANCE} --privileged ${OCID_IMAGE} make binaries
	docker cp ${OCID_INSTANCE}:/go/src/github.com/kubernetes-incubator/cri-o/ocid .
	docker cp ${OCID_INSTANCE}:/go/src/github.com/kubernetes-incubator/cri-o/ocic .
	docker cp ${OCID_INSTANCE}:/go/src/github.com/kubernetes-incubator/cri-o/conmon/conmon ./conmon/conmon
	docker rm ${OCID_INSTANCE}

integration: ocidimage
	docker run -e TESTFLAGS -e TRAVIS -t --privileged --rm -v ${CURDIR}:/go/src/${PROJECT} ${OCID_IMAGE} make localintegration

localintegration: binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: ${OCID_LINK} ocid ocic conmon pause

MANPAGES_MD = $(wildcard docs/*.md)

docs/%.8: docs/%.8.md
	@which go-md2man > /dev/null 2>/dev/null || (echo "ERROR: go-md2man not found. Consider 'make install.tools' target" && false)
	$(GO_MD2MAN) -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@
	gzip -k $@

docs/%.5: docs/%.5.md
	@which go-md2man > /dev/null 2>/dev/null || (echo "ERROR: go-md2man not found. Consider 'make install.tools' target" && false)
	$(GO_MD2MAN) -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@
	gzip -k $@

docs: $(MANPAGES_MD:%.md=%)

install: ocid.conf
	install -D -m 755 ocid ${INSTALLDIR}/ocid
	install -D -m 755 ocic ${INSTALLDIR}/ocic
	install -D -m 755 conmon/conmon $(PREFIX)/libexec/ocid/conmon
	install -D -m 755 pause/pause $(PREFIX)/libexec/ocid/pause
	install -d $(PREFIX)/share/man/man8
	install -m 644 $(wildcard docs/*.8.gz) $(PREFIX)/share/man/man8
	install -d $(PREFIX)/share/man/man5
	install -m 644 $(wildcard docs/*.5.gz) $(PREFIX)/share/man/man5
	install -D -m 644 ocid.service $(PREFIX)/lib/systemd/system
	install -D -m 644 ocid.conf $(DESTDIR)/etc

uninstall:
	systemctl stop ocid.service
	systemctl disable ocid.service
	rm -f $(PREFIX)/lib/systemd/system/ocid.service
	rm -f ${INSTALLDIR}/{ocid,ocic}
	rm -f $(PREFIX)/libexec/ocid/{conmon,pause}
	for i in $(wildcard docs/*.8.gz); do \
		rm -f $(PREFIX)/share/man/man8/$$(basename $${i}); \
	done
	for i in $(wildcard docs/*.5.gz); do \
		rm -f $(PREFIX)/share/man/man5/$$(basename $${i}); \
	done

.PHONY: .gitvalidation
# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make install.tools' target" && false)
ifeq ($(TRAVIS),true)
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
	help \
	install \
	lint \
	ocic \
	ocid \
	pause \
	uninstall
