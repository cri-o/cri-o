include Makefile.inc

export GO111MODULE=off

GO ?= go
EPOCH_TEST_COMMIT ?= 1cc5a27
PROJECT := github.com/cri-o/cri-o
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
GIT_BRANCH_CLEAN := $(shell echo $(GIT_BRANCH) | sed -e "s/[^[:alnum:]]/-/g")
CRIO_IMAGE := crio_dev$(if $(GIT_BRANCH_CLEAN),:$(GIT_BRANCH_CLEAN))
CRIO_INSTANCE := crio_dev
PREFIX ?= ${DESTDIR}/usr/local
BINDIR ?= ${PREFIX}/bin
LIBEXECDIR ?= ${PREFIX}/libexec
MANDIR ?= ${PREFIX}/share/man
ETCDIR ?= ${DESTDIR}/etc
ETCDIR_CRIO ?= ${ETCDIR}/crio
DATAROOTDIR ?= ${PREFIX}/share/containers
BUILDTAGS ?= containers_image_ostree_stub \
			 $(shell hack/apparmor_tag.sh) \
			 $(shell hack/btrfs_installed_tag.sh) \
			 $(shell hack/btrfs_tag.sh) \
			 $(shell hack/libdm_installed.sh) \
			 $(shell hack/libdm_no_deferred_remove_tag.sh) \
			 $(shell hack/openpgp_tag.sh) \
			 $(shell hack/seccomp_tag.sh) \
			 $(shell hack/selinux_tag.sh)
CRICTL_CONFIG_DIR=${DESTDIR}/etc
CONTAINER_RUNTIME ?= podman
BUILD_PATH := $(shell pwd)/build
BUILD_BIN_PATH := ${BUILD_PATH}/bin
COVERAGE_PATH := ${BUILD_PATH}/coverage
JUNIT_PATH := ${BUILD_PATH}/junit
TESTBIN_PATH := ${BUILD_PATH}/test
MOCK_PATH := ${PWD}/test/mocks
MOCKGEN_FLAGS := --build_flags='--tags=test $(BUILDTAGS)'

BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions
OCIUMOUNTINSTALLDIR=$(PREFIX)/share/oci-umount/oci-umount.d

SELINUXOPT ?= $(shell selinuxenabled 2>/dev/null && echo -Z)

BUILD_INFO := $(shell date +%s)

GO_MD2MAN := ${BUILD_BIN_PATH}/go-md2man
GINKGO := ${BUILD_BIN_PATH}/ginkgo
MOCKGEN := ${BUILD_BIN_PATH}/mockgen
GIT_VALIDATION := ${BUILD_BIN_PATH}/git-validation
RELEASE_TOOL := ${BUILD_BIN_PATH}/release-tool
GOLANGCI_LINT := ${BUILD_BIN_PATH}/golangci-lint

NIX_IMAGE := crionix

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
BASE_LDFLAGS := ${SHRINKFLAGS} -X main.gitCommit=${GIT_COMMIT} -X main.buildInfo=${BUILD_INFO}
LDFLAGS := -ldflags '${BASE_LDFLAGS}'

all: binaries crio.conf docs

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

# Dummy target for marking pattern rules phony
.explicit_phony:

.gopathok:
ifeq ("$(wildcard $(GOPKGDIR))","")
	mkdir -p "$(GOPKGBASEDIR)"
	ln -s "$(CURDIR)" "$(GOPKGDIR)"
endif
	touch "$(GOPATH)/.gopathok"

lint: .gopathok ${GOLANGCI_LINT}
	${GOLANGCI_LINT} run

fmt: cfmt

cfmt:
	find . '(' -name '*.h' -o -name '*.c' ')' ! -path './vendor/*'  -exec clang-format -i {} \+
	git diff --exit-code

bin/conmon: conmon/config.h
	$(MAKE) -C conmon

bin/pause:
	$(MAKE) -C pause

test/bin2img/bin2img: .gopathok $(wildcard test/bin2img/*.go)
	$(GO) build $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/bin2img

test/copyimg/copyimg: .gopathok $(wildcard test/copyimg/*.go)
	$(GO) build $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/copyimg

test/checkseccomp/checkseccomp: .gopathok $(wildcard test/checkseccomp/*.go)
	$(GO) build $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/test/checkseccomp

bin/crio: .gopathok
	$(GO) build $(LDFLAGS) -tags "$(BUILDTAGS)" -o $@ $(PROJECT)/cmd/crio

build-static:
	$(CONTAINER_RUNTIME) run --rm -it -v $(shell pwd):/cri-o $(NIX_IMAGE) sh -c \
		"nix-build cri-o/nix --argstr revision $(COMMIT_NO) && \
		mkdir -p cri-o/bin && \
		cp result-*bin/bin/crio-* cri-o/bin && \
		chown -R $(shell id -u):$(shell id -g) cri-o/bin"

nix-image:
	time $(CONTAINER_RUNTIME) build -t $(NIX_IMAGE) \
		--build-arg COMMIT=$(COMMIT_NO) -f Dockerfile-nix .

crio.conf: bin/crio
	./bin/crio --config="" config --default > crio.conf

release-note: ${RELEASE_TOOL}
	${RELEASE_TOOL} -n $(release)

conmon/config.h: cmd/crio-config/config.go oci/oci.go
	$(GO) build $(LDFLAGS) -tags "$(BUILDTAGS)" -o bin/crio-config $(PROJECT)/cmd/crio-config
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
	rm -rf ${BUILD_BIN_PATH}

# the approach here, rather than this target depending on the build targets
# directly, is such that each target should try to build regardless if it
# fails. And return a non-zero exit if _any_ target fails.
local-cross:
	@$(MAKE) --keep-going $(CROSS_BUILD_TARGETS)

bin/crio.cross.%: .gopathok .explicit_phony
	@echo "==> make $@"; \
	TARGET="$*"; \
	GOOS="$${TARGET%%.*}" \
	GOARCH="$${TARGET##*.}" \
	$(GO) build $(LDFLAGS) -tags "containers_image_openpgp btrfs_noversion" -o "$@" $(PROJECT)/cmd/crio

crioimage:
	$(CONTAINER_RUNTIME) build -t ${CRIO_IMAGE} .

dbuild: crioimage
	$(CONTAINER_RUNTIME) run --name=${CRIO_INSTANCE} -e BUILDTAGS --privileged -v ${PWD}:/go/src/${PROJECT} --rm ${CRIO_IMAGE} make binaries

integration: crioimage
	$(CONTAINER_RUNTIME) run -e STORAGE_OPTIONS="--storage-driver=vfs" -e TEST_USERNS -e TESTFLAGS -e TRAVIS -e CRIO_BINARY -t --privileged --rm -v ${CURDIR}:/go/src/${PROJECT} ${CRIO_IMAGE} make localintegration

define go-build
	$(shell cd `pwd` && $(GO) build -o ${BUILD_BIN_PATH}/${1} ${2})
endef

${BUILD_BIN_PATH}:
	mkdir -p ${BUILD_BIN_PATH}

${GO_MD2MAN}: ${BUILD_BIN_PATH}
	$(call go-build,go-md2man,./vendor/github.com/cpuguy83/go-md2man)

${GINKGO}: ${BUILD_BIN_PATH}
	$(call go-build,ginkgo,./vendor/github.com/onsi/ginkgo/ginkgo)

${MOCKGEN}:
	$(call go-build,mockgen,./vendor/github.com/golang/mock/mockgen)

${GIT_VALIDATION}:
	$(call go-build,git-validation,./vendor/github.com/vbatts/git-validation)

${RELEASE_TOOL}:
	$(call go-build,release-tool,./vendor/github.com/containerd/project/cmd/release-tool)

${GOLANGCI_LINT}:
	$(call go-build,golangci-lint,./vendor/github.com/golangci/golangci-lint/cmd/golangci-lint)

vendor:
	export GO111MODULE=on \
		$(GO) mod tidy && \
		$(GO) mod vendor && \
		$(GO) mod verify

testunit: ${GINKGO}
	rm -rf ${COVERAGE_PATH} && mkdir -p ${COVERAGE_PATH}
	rm -rf ${JUNIT_PATH} && mkdir -p ${JUNIT_PATH}
	${BUILD_BIN_PATH}/ginkgo \
		${TESTFLAGS} \
		-r \
		--trace \
		--cover \
		--covermode atomic \
		--outputdir ${COVERAGE_PATH} \
		--coverprofile coverprofile \
		--tags "test $(BUILDTAGS)" \
		--succinct
	# fixes https://github.com/onsi/ginkgo/issues/518
	sed -i '2,$${/^mode: atomic/d;}' ${COVERAGE_PATH}/coverprofile
	$(GO) tool cover -html=${COVERAGE_PATH}/coverprofile -o ${COVERAGE_PATH}/coverage.html
	$(GO) tool cover -func=${COVERAGE_PATH}/coverprofile | sed -n 's/\(total:\).*\([0-9][0-9].[0-9]\)/\1 \2/p'
	find . -name '*_junit.xml' -exec mv -t ${JUNIT_PATH} {} +

testunit-bin:
	mkdir -p ${TESTBIN_PATH}
	for PACKAGE in `$(GO) list ./...`; do \
		go test $$PACKAGE \
			--tags "test $(BUILDTAGS)" \
			--gcflags '-N' -c -o ${TESTBIN_PATH}/$$(basename $$PACKAGE) ;\
	done

mockgen: \
	mock-containerstorage \
	mock-criostorage \
	mock-lib \
	mock-oci \
	mock-sandbox \
	mock-server \
	mock-image-types \
	mock-ocicni-types

mock-containerstorage: ${MOCKGEN}
	${MOCKGEN} \
		${MOCKGEN_FLAGS} \
		-package containerstoragemock \
		-destination ${MOCK_PATH}/containerstorage/containerstorage.go \
		github.com/containers/storage Store

mock-criostorage: ${MOCKGEN}
	${MOCKGEN} \
		${MOCKGEN_FLAGS} \
		-package criostoragemock \
		-destination ${MOCK_PATH}/criostorage/criostorage.go \
		github.com/cri-o/cri-o/pkg/storage ImageServer,RuntimeServer

mock-lib: ${MOCKGEN}
	${MOCKGEN} \
		${MOCKGEN_FLAGS} \
		-package libmock \
		-destination ${MOCK_PATH}/lib/lib.go \
		github.com/cri-o/cri-o/lib ConfigIface

mock-oci: ${MOCKGEN}
	${MOCKGEN} \
		${MOCKGEN_FLAGS} \
		-package ocimock \
		-destination ${MOCK_PATH}/oci/oci.go \
		github.com/cri-o/cri-o/oci RuntimeImpl

mock-sandbox: ${MOCKGEN}
	${MOCKGEN} \
		${MOCKGEN_FLAGS} \
		-package sandboxmock \
		-destination ${MOCK_PATH}/sandbox/sandbox.go \
		github.com/cri-o/cri-o/lib/sandbox NetNsIface

mock-server: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package servermock \
		-destination ${MOCK_PATH}/server/server.go \
		github.com/cri-o/cri-o/server ConfigIface

mock-image-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package imagetypesmock \
		-destination ${MOCK_PATH}/containers/image/types.go \
		github.com/containers/image/types Image

mock-ocicni-types: ${MOCKGEN}
	${BUILD_BIN_PATH}/mockgen \
		${MOCKGEN_FLAGS} \
		-package ocicnitypesmock \
		-destination ${MOCK_PATH}/ocicni/types.go \
		github.com/cri-o/ocicni/pkg/ocicni CNIPlugin

codecov: SHELL := $(shell which bash)
codecov:
	bash <(curl -s https://codecov.io/bash) -f ${COVERAGE_PATH}/coverprofile

localintegration: clean binaries test-binaries
	./test/test_runner.sh ${TESTFLAGS}

binaries: bin/crio bin/conmon bin/pause
test-binaries: test/bin2img/bin2img test/copyimg/copyimg test/checkseccomp/checkseccomp

MANPAGES_MD := $(wildcard docs/*.md)
MANPAGES    := $(MANPAGES_MD:%.md=%)

docs/%.5: docs/%.5.md .gopathok ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

docs/%.8: docs/%.8.md .gopathok ${GO_MD2MAN}
	(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@) || \
		(${GO_MD2MAN} -in $< -out $@.tmp && touch $@.tmp && mv $@.tmp $@)

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
.gitvalidation: .gopathok ${GIT_VALIDATION}
ifeq ($(TRAVIS),true)
	GIT_CHECK_EXCLUDE="./vendor" ${GIT_VALIDATION} -q -run DCO,short-subject,dangling-whitespace
else
	GIT_CHECK_EXCLUDE="./vendor" ${GIT_VALIDATION} -v -run DCO,short-subject,dangling-whitespace -range $(EPOCH_TEST_COMMIT)..HEAD
endif

.PHONY: \
	.explicit_phony \
	.gitvalidation \
	bin/conmon \
	bin/crio \
	bin/pause \
	binaries \
	build-static \
	clean \
	default \
	docs \
	help \
	install \
	lint \
	local-cross \
	nix-image \
	uninstall \
	vendor
