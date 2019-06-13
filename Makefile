GO ?= go
EPOCH_TEST_COMMIT ?= 1cc5a27
PROJECT := github.com/cri-o/cri-o
CRIO_IMAGE = crio_dev$(if $(GIT_BRANCH_CLEAN),:$(GIT_BRANCH_CLEAN))
CRIO_INSTANCE := crio_dev
PREFIX ?= ${DESTDIR}/usr/local
BINDIR ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR ?= ${PREFIX}/share/man
ETCDIR ?= ${DESTDIR}/etc
ETCDIR_CRIO ?= ${ETCDIR}/crio
DATAROOTDIR ?= ${PREFIX}/share/containers
BUILDTAGS ?= $(shell hack/btrfs_tag.sh) $(shell hack/libdm_installed.sh) $(shell hack/libdm_no_deferred_remove_tag.sh) $(shell hack/btrfs_installed_tag.sh) $(shell hack/ostree_tag.sh) $(shell hack/seccomp_tag.sh) $(shell hack/selinux_tag.sh) $(shell hack/apparmor_tag.sh)
CRICTL_CONFIG_DIR=${DESTDIR}/etc
CONTAINER_RUNTIME ?= podman
BUILD_PATH := ${PWD}/build
BUILD_BIN_PATH := ${BUILD_PATH}/bin
COVERAGE_PATH := ${BUILD_PATH}/coverage
TESTBIN_PATH := ${BUILD_PATH}/test
MOCK_PATH := ${PWD}/test/mocks
MOCKGEN_FLAGS := --build_flags='--tags=containers_image_ostree_stub $(BUILDTAGS)'

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
OCIUMOUNTINSTALLDIR=$(PREFIX)/share/oci-umount/oci-umount.d

SELINUXOPT ?= $(shell selinuxenabled 2>/dev/null && echo -Z)

BUILD_INFO := $(shell date +%s)

CROSS_BUILD_TARGETS := \
	bin/crio.cross.windows.amd64 \
	bin/crio.cross.darwin.amd64 \
	bin/crio.cross.linux.amd64

# If GOPATH not specified, use one in the local directory
ifeq ($(GOPATH),)
export GOPATH := $(CURDIR)/_output
unexport GOBIN
endif
GOPKGDIR := $(GOPATH)/src/$(PROJECT)
GOPKGBASEDIR := $(shell dirname "$(GOPKGDIR)")

# Update VPATH so make finds .gopathok
VPATH := $(VPATH):$(GOPATH)
SHRINKFLAGS := -s -w
BASE_LDFLAGS = ${SHRINKFLAGS} -X main.gitCommit=${GIT_COMMIT} -X main.buildInfo=${BUILD_INFO}
LDFLAGS = -ldflags '${BASE_LDFLAGS}'

all: binaries crio.conf docs

include Makefile.inc

default: help

help:
	@echo "Usage: make <target>"
	@echo
	@echo " * 'install' - Install binaries to system locations"
	@echo " * 'binaries' - Build crio, conmon and pause"
	@echo " * 'release-note' - Generate release note"
	@echo " * 'integration' - Execute integration tests"
	@echo " * 'clean' - Clean artifacts"
	@echo " * 'lint' - Execute the source code linter"
	@echo " * 'gofmt' - Verify the source code gofmt"

# Dummy target for marking pattern rules phony
.explicit_phony:

.gopathok:
ifeq ("$(wildcard $(GOPKGDIR))","")
	mkdir -p "$(GOPKGBASEDIR)"
	ln -s "$(CURDIR)" "$(GOPKGDIR)"
endif
	touch "$(GOPATH)/.gopathok"

lint: .gopathok
	golangci-lint run --build-tags="$(BUILDTAGS) containers_image_ostree_stub"

fmt: gofmt cfmt

cfmt:
	find . '(' -name '*.h' -o -name '*.c' ')' ! -path './vendor/*'  -exec clang-format -i {} \+
	git diff --exit-code

gofmt:
	find . -name '*.go' ! -path './vendor/*' -exec gofmt -s -w {} \+
	git diff --exit-code

bin/conmon: conmon/config.h
	$(MAKE) -C conmon

bin/pause:
	$(MAKE) -C pause

test/bin2img/bin2img: git-vars .gopathok $(wildcard test/bin2img/*.go)
	$(GO) build -i $(LDFLAGS) -tags "$(BUILDTAGS) containers_image_ostree_stub" -o $@ $(PROJECT)/test/bin2img

test/copyimg/copyimg: git-vars .gopathok $(wildcard test/copyimg/*.go)
	$(GO) build -i $(LDFLAGS) -tags "$(BUILDTAGS) containers_image_ostree_stub" -o $@ $(PROJECT)/test/copyimg

test/checkseccomp/checkseccomp: git-vars .gopathok $(wildcard test/checkseccomp/*.go)
	$(GO) build -i $(LDFLAGS) -tags "$(BUILDTAGS) containers_image_ostree_stub" -o $@ $(PROJECT)/test/checkseccomp

bin/crio: git-vars .gopathok
	$(GO) build -i $(LDFLAGS) -tags "$(BUILDTAGS) containers_image_ostree_stub" -o $@ $(PROJECT)/cmd/crio

crio.conf: bin/crio
	./bin/crio --config="" config --default > crio.conf

release-note:
	@$(GOPATH)/bin/release-tool -n $(release)

conmon/config.h: git-vars cmd/crio-config/config.go oci/oci.go
	$(GO) build -i $(LDFLAGS) -tags "$(BUILDTAGS)" -o bin/crio-config $(PROJECT)/cmd/crio-config
	( cd conmon && $(CURDIR)/bin/crio-config )

clean:
ifneq ($(GOPATH),)
	rm -f "$(GOPATH)/.gopathok"
endif
	rm -rf _output
	rm -f docs/*.5 docs/*.8
	rm -fr test/testdata/redis-image
	find . -name \*~ -delete
	find . -name \#\* -delete
	rm -f bin/crio
	rm -f bin/crio.cross.*
	$(MAKE) -C conmon clean
	$(MAKE) -C pause clean
	rm -f test/bin2img/bin2img
	rm -f test/copyimg/copyimg
	rm -f test/checkseccomp/checkseccomp

# the approach here, rather than this target depending on the build targets
# directly, is such that each target should try to build regardless if it
# fails. And return a non-zero exit if _any_ target fails.
local-cross:
	@$(MAKE) --keep-going $(CROSS_BUILD_TARGETS)

bin/crio.cross.%: git-vars .gopathok .explicit_phony
	@echo "==> make $@"; \
	TARGET="$*"; \
	GOOS="$${TARGET%%.*}" \
	GOARCH="$${TARGET##*.}" \
	$(GO) build -i $(LDFLAGS) -tags "containers_image_openpgp btrfs_noversion" -o "$@" $(PROJECT)/cmd/crio

crioimage: git-vars
	$(CONTAINER_RUNTIME) build -t ${CRIO_IMAGE} .

dbuild: crioimage
	$(CONTAINER_RUNTIME) run --name=${CRIO_INSTANCE} -e BUILDTAGS --privileged -v ${PWD}:/go/src/${PROJECT} --rm ${CRIO_IMAGE} make binaries

integration: crioimage
	$(CONTAINER_RUNTIME) run -e STORAGE_OPTIONS="--storage-driver=vfs" -e TEST_USERNS -e TESTFLAGS -e TRAVIS -t --privileged --rm -v ${CURDIR}:/go/src/${PROJECT} ${CRIO_IMAGE} make localintegration

${BUILD_BIN_PATH}/ginkgo:
	mkdir -p ${BUILD_BIN_PATH}
	$(GO) build -o ${BUILD_BIN_PATH}/ginkgo ./vendor/github.com/onsi/ginkgo/ginkgo

vendor: .install.vndr
	$(GOPATH)/bin/vndr \
		-whitelist "github.com/onsi/ginkgo" \
		-whitelist "github.com/golang/mock" \
		${PKG}

${BUILD_BIN_PATH}/mockgen:
	mkdir -p ${BUILD_BIN_PATH}
	$(GO) build -o ${BUILD_BIN_PATH}/mockgen ./vendor/github.com/golang/mock/mockgen

testunit: mockgen ${BUILD_BIN_PATH}/ginkgo
	rm -rf ${COVERAGE_PATH} && mkdir -p ${COVERAGE_PATH}
	${BUILD_BIN_PATH}/ginkgo \
		${TESTFLAGS} \
		-r \
		--cover \
		--covermode atomic \
		--outputdir ${COVERAGE_PATH} \
		--coverprofile coverprofile \
		--tags "containers_image_ostree_stub $(BUILDTAGS)" \
		--succinct
	# fixes https://github.com/onsi/ginkgo/issues/518
	sed -i '2,$${/^mode: atomic/d;}' ${COVERAGE_PATH}/coverprofile
	$(GO) tool cover -html=${COVERAGE_PATH}/coverprofile -o ${COVERAGE_PATH}/coverage.html
	$(GO) tool cover -func=${COVERAGE_PATH}/coverprofile | sed -n 's/\(total:\).*\([0-9][0-9].[0-9]\)/\1 \2/p'

testunit-bin:
	mkdir -p ${TESTBIN_PATH}
	for PACKAGE in ${PACKAGES}; do \
		go test $$PACKAGE \
			--tags "containers_image_ostree_stub $(BUILDTAGS)" \
			--gcflags '-N' -c -o ${TESTBIN_PATH}/$$(basename $$PACKAGE) ;\
	done

mockgen: ${BUILD_BIN_PATH}/mockgen
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package containerstoragemock \
		-destination ${MOCK_PATH}/containerstorage/containerstorage.go \
		github.com/containers/storage Store
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package criostoragemock \
		-destination ${MOCK_PATH}/criostorage/criostorage.go \
		github.com/cri-o/cri-o/pkg/storage ImageServer
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package libmock \
		-destination ${MOCK_PATH}/lib/lib.go \
		github.com/cri-o/cri-o/lib ConfigIface
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package ocimock \
		-destination ${MOCK_PATH}/oci/oci.go \
		github.com/cri-o/cri-o/oci RuntimeImpl
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package sandboxmock \
		-destination ${MOCK_PATH}/sandbox/sandbox.go \
		github.com/cri-o/cri-o/lib/sandbox NetNsIface


codecov: SHELL := $(shell which bash)
codecov:
	bash <(curl -s https://codecov.io/bash) -f ${COVERAGE_PATH}/coverprofile

localintegration: clean binaries test-binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: bin/crio bin/conmon bin/pause
test-binaries: test/bin2img/bin2img test/copyimg/copyimg test/checkseccomp/checkseccomp

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

docs/%.5: docs/%.5.md .gopathok
	(go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || ($(GOPATH)/bin/go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs/%.8: docs/%.8.md .gopathok
	(go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || ($(GOPATH)/bin/go-md2man -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs: $(MANPAGES)

install: .gopathok install.bin install.man

install.bin: binaries
	install ${SELINUXOPT} -D -m 755 bin/crio $(BINDIR)/crio
	install ${SELINUXOPT} -D -m 755 bin/conmon $(LIBEXECDIR)/crio/conmon
	install ${SELINUXOPT} -D -m 755 bin/pause $(LIBEXECDIR)/crio/pause

install.man: $(MANPAGES)
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man5
	install ${SELINUXOPT} -d -m 755 $(MANDIR)/man8
	install ${SELINUXOPT} -m 644 $(filter %.5,$(MANPAGES)) -t $(MANDIR)/man5
	install ${SELINUXOPT} -m 644 $(filter %.8,$(MANPAGES)) -t $(MANDIR)/man8

install.config: crio.conf
	install ${SELINUXOPT} -d $(DATAROOTDIR)/oci/hooks.d
	install ${SELINUXOPT} -D -m 644 crio.conf $(ETCDIR_CRIO)/crio.conf
	install ${SELINUXOPT} -D -m 644 seccomp.json $(ETCDIR_CRIO)/seccomp.json
	install ${SELINUXOPT} -D -m 644 crio-umount.conf $(OCIUMOUNTINSTALLDIR)/crio-umount.conf
	install ${SELINUXOPT} -D -m 644 crictl.yaml $(CRICTL_CONFIG_DIR)

install.completions:
	install ${SELINUXOPT} -d -m 755 ${BASHINSTALLDIR}

install.systemd:
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio.service $(PREFIX)/lib/systemd/system/crio.service
	ln -sf crio.service $(PREFIX)/lib/systemd/system/cri-o.service
	install ${SELINUXOPT} -D -m 644 contrib/systemd/crio-shutdown.service $(PREFIX)/lib/systemd/system/crio-shutdown.service

uninstall:
	rm -f $(BINDIR)/crio
	rm -f $(LIBEXECDIR)/crio/conmon
	rm -f $(LIBEXECDIR)/crio/pause
	for i in $(filter %.5,$(MANPAGES)); do \
		rm -f $(MANDIR)/man5/$$(basename $${i}); \
	done
	for i in $(filter %.8,$(MANPAGES)); do \
		rm -f $(MANDIR)/man8/$$(basename $${i}); \
	done

# When this is running in travis, it will only check the travis commit range
.gitvalidation: .gopathok
ifeq ($(TRAVIS),true)
	GIT_CHECK_EXCLUDE="./vendor" $(GOPATH)/bin/git-validation -q -run DCO,short-subject,dangling-whitespace
else
	GIT_CHECK_EXCLUDE="./vendor" $(GOPATH)/bin/git-validation -v -run DCO,short-subject,dangling-whitespace -range $(EPOCH_TEST_COMMIT)..HEAD
endif

install.tools: .install.gitvalidation .install.golangci-lint .install.md2man .install.release

.install.release:
	if [ ! -x "$(GOPATH)/bin/release-tool" ]; then \
		go get -u github.com/containerd/project/cmd/release-tool; \
	fi

.install.gitvalidation: .gopathok
	if [ ! -x "$(GOPATH)/bin/git-validation" ]; then \
		go get -u github.com/vbatts/git-validation; \
	fi

.install.golangci-lint: .gopathok
	if [ ! -x "$(GOPATH)/bin/golangci-lint" ]; then \
		go get -u github.com/golangci/golangci-lint/cmd/golangci-lint; \
	fi

.install.md2man: .gopathok
	if [ ! -x "$(GOPATH)/bin/go-md2man" ]; then \
		go get -u github.com/cpuguy83/go-md2man; \
	fi

.install.vndr: .gopathok
	$(GO) get -u github.com/LK4D4/vndr

.install.ostree: .gopathok
	if ! pkg-config ostree-1 2> /dev/null ; then \
		git clone https://github.com/ostreedev/ostree $(GOPATH)/src/github.com/ostreedev/ostree ; \
		cd $(GOPATH)/src/github.com/ostreedev/ostree ; \
		./autogen.sh --prefix=/usr/local; \
		$(MAKE) all install; \
	fi

.PHONY: \
	.explicit_phony \
	.gitvalidation \
	bin/conmon \
	bin/crio \
	bin/pause \
	binaries \
	clean \
	default \
	docs \
	gofmt \
	help \
	install \
	install.tools \
	lint \
	local-cross \
	uninstall \
	vendor
